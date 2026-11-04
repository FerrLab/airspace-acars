package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// defaultRepeatEvery is the boarding repeat interval for a line that does
// not give one: the editor's own default.
const defaultRepeatEvery = 5 * time.Minute

// errPackNotLoaded refuses a manual play before the pack is in.
var errPackNotLoaded = errors.New("the speech pack is not loaded")

// play fetches what a fired phase plays and queues it, all of it at once so
// the phase's lines play back to back. during is the flight's step when it
// fired; a line the server could not voice yet is asked for again while the
// flight is still there.
func (e *attendant) play(gen int, f phaseFire, during string) {
	_, span := observability.Start(context.Background(), "attendant.play_phase", "attendant.phase", f.phase)
	defer span.Finish()

	e.mu.Lock()
	pack, arrival := e.pack, e.packArrival
	e.mu.Unlock()
	if pack == nil {
		return
	}

	lines, fresh := e.linesFor(pack, f, arrival)
	var clips []queuedClip
	for _, l := range lines {
		rendered, segs, err := e.prepare(l, arrival, fresh)
		if err != nil {
			e.lineNotReady(gen, lineRetry{line: l, fire: f, fresh: fresh, during: during}, err)
			continue
		}
		clips = append(clips, clipFor(rendered, f, segs))
	}
	span.Set("attendant.clips", len(clips))
	e.enqueue(gen, clips, false)
}

// linesFor picks the lines a fire plays. After landing they come from the
// server for the touchdown that happened; fresh says they were rendered now.
func (e *attendant) linesFor(pack *speechPack, f phaseFire, arrival string) (lines []speechLine, fresh bool) {
	switch f.phase {
	case domain.PhaseAfterLanding:
		got, err := fetchSpeechPhase(e.deps.api, domain.PhaseAfterLanding, lineQuery{arrival: arrival, landing: f.landing})
		if err == nil {
			return got, true
		}
		slog.Warn("attendant: after-landing lines could not be read; using the pack", "error", err)
		for _, l := range pack.playable(f.phase) {
			if c := l.Conditions.Landing; c == "" || c == "any" || (c == f.landing && f.landing != domain.LandingNormal) {
				lines = append(lines, l)
			}
		}
		return lines, false
	case domain.PhaseBoarding:
		for _, l := range pack.playable(f.phase) {
			if !l.isMusic() {
				lines = append(lines, l)
			}
		}
		return lines, false
	case domain.PhaseCruise:
		for _, l := range pack.playable(f.phase) {
			if l.cruiseAt() == f.cruiseAt {
				lines = append(lines, l)
			}
		}
		return lines, false
	case domain.PhaseMealService:
		for _, l := range pack.playable(f.phase) {
			if f.service == 0 || l.service() == f.service {
				lines = append(lines, l)
			}
		}
		return lines, false
	}
	return pack.playable(f.phase), false
}

// prepare returns a line as it plays, with its audio as cached files and
// pauses. A dynamic line, or one whose audio the server has not generated
// yet, is rendered again first, unless fresh says it just was; its text is
// then the one rendered now.
func (e *attendant) prepare(l speechLine, arrival string, fresh bool) (speechLine, []domain.SoundInstruction, error) {
	key := lineKey(l)
	if !l.Dynamic {
		e.mu.Lock()
		segs, ok := e.prepared[key]
		e.mu.Unlock()
		if ok {
			return l, segs, nil
		}
	}
	if !fresh && (l.Dynamic || !l.hasAudio()) {
		got, err := fetchSpeechLine(e.deps.api, l.id(), lineQuery{arrival: arrival, service: l.service()})
		if err != nil {
			return l, nil, err
		}
		if got.Phase == "" {
			got.Phase = l.Phase
		}
		got.Conditions.Repeat = got.Conditions.Repeat || l.Conditions.Repeat
		l = got
	}
	segs, err := e.download(l)
	if err != nil {
		return l, nil, err
	}
	if !l.Dynamic {
		e.mu.Lock()
		e.prepared[key] = segs
		e.mu.Unlock()
	}
	return l, segs, nil
}

// download caches a line's audio: its combined file when the server made
// one, its segments otherwise.
func (e *attendant) download(l speechLine) ([]domain.SoundInstruction, error) {
	if l.AudioURL != "" {
		name, err := e.deps.download(l.AudioURL)
		if err != nil {
			return nil, err
		}
		return []domain.SoundInstruction{{Type: "play", LocalFile: name}}, nil
	}
	var out []domain.SoundInstruction
	for _, s := range l.Segments {
		switch {
		case s.Type == "play" && s.URL != "":
			name, err := e.deps.download(s.URL)
			if err != nil {
				return nil, err
			}
			out = append(out, domain.SoundInstruction{Type: "play", LocalFile: name})
		case s.Type == "pause" && s.DurationMs > 0:
			out = append(out, domain.SoundInstruction{Type: "pause", DurationMs: int(s.DurationMs)})
		}
	}
	if len(out) == 0 {
		return nil, errLineUnvoiced
	}
	return out, nil
}

// lineFailed records a line that could not be prepared. One no longer in
// the pack means the pack changed under the flight, so it is read again.
func (e *attendant) lineFailed(gen int, l speechLine, f phaseFire, err error) {
	if observability.Transient(err) || errors.Is(err, errLineGone) {
		slog.Debug("attendant: line not played", "attendant.line", l.id(), "error", err)
	} else {
		slog.Warn("attendant: line not played", "attendant.line", l.id(), "error", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gen != gen {
		return
	}
	e.history = appendHistory(e.history, failedEntry(e.deps.now(), l, f))
	if errors.Is(err, errLineGone) {
		e.sendLocked(attendantJob{gen: gen, kind: jobLoad})
	}
}

func clipFor(l speechLine, f phaseFire, segs []domain.SoundInstruction) queuedClip {
	service := l.service()
	if service == 0 {
		service = f.service
	}
	c := queuedClip{AttendantClip: domain.AttendantClip{
		LineID: l.id(), Phase: f.phase, Service: service, Text: l.Text,
		Language: l.Language, Voice: l.Voice, Music: l.isMusic(), Reason: f.reason, Segments: segs,
	}}
	if f.phase == domain.PhaseBoarding && l.Conditions.Repeat && !l.isMusic() && f.reason.Code != domain.ReasonManual {
		c.repeat = true
		c.repeatEvery = minutes(l.Conditions.RepeatEveryMinutes)
		if c.repeatEvery <= 0 {
			c.repeatEvery = defaultRepeatEvery
		}
	}
	return c
}

// enqueue adds clips to the end of the queue. Boarding music playing is cut
// short for them: it only fills the silence between announcements. A
// boarding repeat also brings back music the pilot skipped.
func (e *attendant) enqueue(gen int, clips []queuedClip, resumeMusic bool) {
	if len(clips) == 0 {
		return
	}
	e.mu.Lock()
	if e.gen != gen || !e.enabled {
		e.mu.Unlock()
		return
	}
	for i := range clips {
		e.clipSeq++
		clips[i].ID = "clip-" + strconv.Itoa(e.clipSeq)
	}
	e.queue = append(e.queue, clips...)
	if resumeMusic {
		e.musicOff = false
	}
	var cut string
	if e.playing != nil && e.playing.Music {
		cut = e.playing.ID
		e.recordLocked(*e.playing, domain.ClipInterrupted)
		e.playing = nil
	}
	e.mu.Unlock()

	e.interrupt(cut)
	e.clipReady()
	e.notify()
}

// repeat queues a boarding line again.
func (e *attendant) repeat(gen int, l speechLine) {
	e.mu.Lock()
	arrival := e.packArrival
	e.mu.Unlock()

	f := phaseFire{phase: domain.PhaseBoarding, reason: domain.AttendantReason{
		Code: domain.ReasonBoardingRepeat, Minutes: int(minutes(l.Conditions.RepeatEveryMinutes).Minutes())}}
	if f.reason.Minutes == 0 {
		f.reason.Minutes = int(defaultRepeatEvery.Minutes())
	}
	rendered, segs, err := e.prepare(l, arrival, false)
	if err != nil {
		e.lineFailed(gen, l, f, err)
		// Try again at the next interval rather than never.
		e.mu.Lock()
		if e.gen == gen {
			e.repeatDue[l.id()] = e.deps.now().Add(repeatEveryFor(l))
		}
		e.mu.Unlock()
		return
	}
	e.enqueue(gen, []queuedClip{clipFor(rendered, f, segs)}, true)
}

func repeatEveryFor(l speechLine) time.Duration {
	if d := minutes(l.Conditions.RepeatEveryMinutes); d > 0 {
		return d
	}
	return defaultRepeatEvery
}

// boardingLocked reports whether boarding is under way: announced and not
// yet complete. Callers hold e.mu.
func (e *attendant) boardingLocked() bool {
	d := e.detector
	return d != nil && d.has(domain.PhaseBoarding) && !d.passed(domain.PhaseBoardingComplete)
}

// repeatsLocked queues the boarding lines whose repeat is due. Callers hold
// e.mu.
func (e *attendant) repeatsLocked(now time.Time) {
	if !e.enabled || e.pack == nil || !e.boardingLocked() {
		return
	}
	for id, due := range e.repeatDue {
		if now.Before(due) {
			continue
		}
		delete(e.repeatDue, id)
		for _, l := range e.pack.playable(domain.PhaseBoarding) {
			if l.id() == id {
				e.sendLocked(attendantJob{gen: e.gen, kind: jobRepeat, line: l})
				break
			}
		}
	}
}

// endBoardingLocked stops the boarding loop: no more repeats, no more music.
// It returns the music clip to cut, if one is playing. Callers hold e.mu.
func (e *attendant) endBoardingLocked() string {
	e.repeatDue = map[int]time.Time{}
	e.musicOff = true
	kept := e.queue[:0]
	for _, c := range e.queue {
		if !c.Music && c.Reason.Code != domain.ReasonBoardingRepeat {
			kept = append(kept, c)
		}
	}
	e.queue = kept
	if e.playing != nil && e.playing.Music {
		id := e.playing.ID
		e.recordLocked(*e.playing, domain.ClipInterrupted)
		e.playing = nil
		return id
	}
	return ""
}

// nextMusicLocked returns the next boarding music clip when boarding is
// under way and nothing else is waiting. Callers hold e.mu.
func (e *attendant) nextMusicLocked() *queuedClip {
	// An announcement on its way from the server comes first.
	if e.musicOff || e.pending > 0 || e.pack == nil || !e.boardingLocked() {
		return nil
	}
	var music []speechLine
	for _, l := range e.pack.playable(domain.PhaseBoarding) {
		if _, ok := e.prepared[lineKey(l)]; ok && l.isMusic() {
			music = append(music, l)
		}
	}
	if len(music) == 0 {
		return nil
	}
	l := music[e.musicIdx%len(music)]
	e.musicIdx++
	e.clipSeq++
	c := clipFor(l, phaseFire{phase: domain.PhaseBoarding,
		reason: domain.AttendantReason{Code: domain.ReasonBoardingMusic}}, e.prepared[lineKey(l)])
	c.ID = "clip-" + strconv.Itoa(e.clipSeq)
	return &c
}

// prefetchOne downloads the next line of the pack, and reports whether
// there was one.
func (e *attendant) prefetchOne(gen int) bool {
	e.mu.Lock()
	if e.gen != gen || len(e.prefetch) == 0 {
		e.mu.Unlock()
		return false
	}
	l := e.prefetch[0]
	e.prefetch = e.prefetch[1:]
	arrival := e.packArrival
	e.mu.Unlock()

	if _, _, err := e.prepare(l, arrival, true); err != nil {
		slog.Debug("attendant: prefetch failed", "attendant.line", l.id(), "error", err)
		return true
	}
	if l.isMusic() {
		e.mu.Lock()
		idle := e.gen == gen && e.enabled && e.playing == nil && len(e.queue) == 0 && e.boardingLocked()
		e.mu.Unlock()
		if idle {
			e.clipReady()
		}
	}
	return true
}

// TakeClip hands the player the next clip, or nil when there is none. A
// clip still marked playing was lost by the player (the webview reloaded)
// and is recorded as interrupted.
func (e *attendant) TakeClip() *domain.AttendantClip {
	e.mu.Lock()
	if e.mode != domain.AttendantClient || !e.enabled {
		e.mu.Unlock()
		return nil
	}
	if e.playing != nil {
		e.recordLocked(*e.playing, domain.ClipInterrupted)
		e.playing = nil
	}
	var next *queuedClip
	if len(e.queue) > 0 {
		c := e.queue[0]
		e.queue = e.queue[1:]
		next = &c
	} else {
		next = e.nextMusicLocked()
	}
	e.playing = next
	e.mu.Unlock()

	e.notify()
	if next == nil {
		return nil
	}
	out := next.AttendantClip
	return &out
}

// ClipEnded is told how the clip the player had ended: played or failed.
// A boarding repeat line is timed from here.
func (e *attendant) ClipEnded(id string, played bool) {
	e.mu.Lock()
	if e.playing == nil || e.playing.ID != id {
		e.mu.Unlock()
		return
	}
	c := *e.playing
	e.playing = nil
	outcome := domain.ClipFailed
	if played {
		outcome = domain.ClipPlayed
		if !c.Music {
			e.lastPlayed = &c
		}
	}
	e.recordLocked(c, outcome)
	if c.repeat && e.boardingLocked() {
		e.repeatDue[c.LineID] = e.deps.now().Add(c.repeatEvery)
	}
	e.mu.Unlock()
	e.notify()
}

// Skip cuts the current clip short. Skipping the boarding music stops it
// until the next boarding announcement.
func (e *attendant) Skip() {
	e.mu.Lock()
	var cut string
	if c := e.playing; c != nil {
		cut = c.ID
		if c.Music {
			e.musicOff = true
		}
		if c.repeat && e.boardingLocked() {
			e.repeatDue[c.LineID] = e.deps.now().Add(c.repeatEvery)
		}
		e.recordLocked(*c, domain.ClipSkipped)
		e.playing = nil
	}
	e.mu.Unlock()
	e.interrupt(cut)
	e.clipReady()
	e.notify()
}

// StopAll silences the cabin: the current clip is cut, the queue emptied,
// and the boarding loop ended. Phases still to come play as they fire.
func (e *attendant) StopAll() {
	e.mu.Lock()
	cut := e.clearLocked(domain.ClipStopped)
	e.repeatDue = map[int]time.Time{}
	e.musicOff = true
	e.mu.Unlock()
	e.interrupt(cut)
	e.notify()
}

// clearLocked empties the queue and ends the current clip with outcome,
// returning its id. Callers hold e.mu.
func (e *attendant) clearLocked(outcome string) string {
	e.queue = nil
	if e.playing == nil {
		return ""
	}
	id := e.playing.ID
	e.recordLocked(*e.playing, outcome)
	e.playing = nil
	return id
}

// Replay queues the last line that played to the end once more.
func (e *attendant) Replay() {
	e.mu.Lock()
	last, gen := e.lastPlayed, e.gen
	e.mu.Unlock()
	if last == nil {
		return
	}
	c := *last
	c.repeat = false
	c.Reason = domain.AttendantReason{Code: domain.ReasonReplay}
	e.enqueue(gen, []queuedClip{c}, false)
}

// PlayPhaseNow plays a phase's lines at once, for trying a pack out. It
// does not count as the phase firing.
func (e *attendant) PlayPhaseNow(phase string, service int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mode != domain.AttendantClient || e.pack == nil {
		return errPackNotLoaded
	}
	f := phaseFire{phase: phase, service: service, reason: domain.AttendantReason{Code: domain.ReasonManual}}
	if phase == domain.PhaseAfterLanding {
		f.landing = domain.LandingNormal
	}
	e.sendLocked(attendantJob{gen: e.gen, kind: jobFire, fire: f, during: e.current})
	return nil
}

// recordLocked adds a finished clip to the history. Callers hold e.mu.
func (e *attendant) recordLocked(c queuedClip, outcome string) {
	e.history = appendHistory(e.history, domain.AttendantHistoryEntry{
		At: e.deps.now(), Phase: c.Phase, Service: c.Service, Text: c.Text, Language: c.Language,
		Voice: c.Voice, Music: c.Music, Outcome: outcome, Reason: c.Reason})
}
