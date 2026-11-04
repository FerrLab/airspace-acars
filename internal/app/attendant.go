package app

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// Events the flight attendant sends the webview. Registered in main.go.
const (
	// attendantClipReadyEvent tells the player a clip is waiting: it calls
	// TakeAttendantClip until there is none.
	attendantClipReadyEvent = "attendant-clip-ready"
	// attendantInterruptEvent names a clip the player must cut short.
	attendantInterruptEvent = "attendant-interrupt"
	// attendantStateEvent carries a fresh snapshot after anything the tab
	// shows has changed, other than the aircraft's own readings.
	attendantStateEvent = "attendant-state"
)

const (
	// attendantJobDepth is how many fired phases may wait on the worker. A
	// flight fires about twenty; a full queue means the worker is stuck on
	// the network, and dropping a phase then beats blocking the sampler.
	attendantJobDepth = 64
	// attendantHistoryMax is how much of the flight's playback the tab keeps.
	attendantHistoryMax = 40
)

// packRetryEvery is the pause before the pack is read again after a failure
// that was not an answer: the network, or the server having trouble. A
// variable so a test can run the retry without waiting it out.
var packRetryEvery = 30 * time.Second

// attendantDeps is everything the flight attendant needs from outside.
type attendantDeps struct {
	api      AirspaceAPI
	download func(url string) (string, error)
	emit     func(name string, data interface{})
	gsx      func() domain.GSXState
	now      func() time.Time
}

type jobKind int

const (
	jobLoad jobKind = iota
	jobFire
	jobRepeat
	jobArrival
	jobRetry
)

// attendantJob is network work for the worker, tagged with the flight it
// belongs to so work left over from a flight that ended is dropped.
type attendantJob struct {
	gen  int
	kind jobKind
	fire phaseFire
	// during is the flight's step when fire happened (jobFire), which a
	// line that has to be asked for again must still be at to play.
	during  string
	line    speechLine // jobRepeat
	arrival string     // jobArrival
	retry   lineRetry  // jobRetry
}

// queuedClip is a clip with what the attendant needs to know once it ends.
type queuedClip struct {
	domain.AttendantClip
	// repeat marks a boarding line that plays again once
	// repeat_every_minutes have passed since it last finished.
	repeat      bool
	repeatEvery time.Duration
}

// attendant is the flight attendant: it reads the airline's speech pack when
// a flight starts, decides from the aircraft, GSX and the booking when each
// phase of the flight happens, fetches the lines for it, and hands them to
// the webview to play in order.
//
// The webview owns audio output, so playback is a pull: the attendant keeps
// the queue, says when a clip is ready, and is told when each one ends,
// which is what boarding repeats are timed from.
//
// Readings come in through Observe from the data stream and the flare
// sampler; network work goes to one worker goroutine per flight, so phases
// are fetched and queued in the order they fired.
type attendant struct {
	deps attendantDeps

	// mu guards every field below. Nothing is called out to while it is
	// held: no request, no emit, no other lock.
	mu sync.Mutex

	enabled bool
	// gen counts flights; work for an earlier one is dropped.
	gen  int
	mode string
	// loadErr is why the pack could not be read, "" once it was, and
	// loadFailures how many reads in a row have failed.
	loadErr      string
	loadFailures int

	bookingID string
	start     time.Time
	// packArrival is the arrival the pack was read for: "" for the
	// booking's own, the alternate after a diversion.
	packArrival string
	pack        *speechPack
	detector    *phaseDetector

	// prepared holds the cached segments of every line whose audio is on
	// disk, by lineKey, for the pack's current revision.
	prepared map[string][]domain.SoundInstruction
	// prefetch is the lines still to download, in flight order.
	prefetch []speechLine

	queue   []queuedClip
	playing *queuedClip
	// lastPlayed is the last line that played to the end, for Replay.
	lastPlayed *queuedClip
	history    []domain.AttendantHistoryEntry
	clipSeq    int

	// repeatDue is when each boarding repeat line next plays, by line id;
	// absent while it is queued or playing.
	repeatDue map[int]time.Time
	// musicOff stops the boarding music: the pilot stopped or skipped it.
	// The next boarding announcement brings it back.
	musicOff bool
	musicIdx int
	// pending counts announcements handed to the worker and not yet queued;
	// the boarding music waits for them.
	pending int
	// retries are the lines the server could not voice yet, waiting to be
	// asked for again; Observe hands each to the worker when it is due.
	retries []lineRetry

	reasons  map[string]domain.AttendantReason
	counts   map[string]int
	current  string
	gsxState domain.GSXState
	seatbelt domain.CabinData

	jobs chan attendantJob
	stop chan struct{}
}

func newAttendant(deps attendantDeps) *attendant {
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.gsx == nil {
		deps.gsx = func() domain.GSXState { return domain.GSXState{} }
	}
	return &attendant{deps: deps, enabled: true, mode: domain.AttendantIdle}
}

// SetEnabled turns the attendant on or off. Turning it off cuts what is
// playing and empties the queue; phases go on being followed, so turning it
// back on carries on from where the flight is.
func (e *attendant) SetEnabled(on bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.enabled = on
	var cut string
	if !on {
		cut = e.clearLocked(domain.ClipStopped)
		e.retries = nil
	}
	e.mu.Unlock()
	e.interrupt(cut)
	e.notify()
}

// Mode is how this flight's announcements are played. A nil attendant
// (an App built without NewApp, in tests) is idle.
func (e *attendant) Mode() string {
	if e == nil {
		return domain.AttendantIdle
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode
}

// Start begins a flight: the pack is read and phases are followed from the
// next reading. It does not block.
func (e *attendant) Start(bookingID string, start time.Time) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.stopLocked()
	e.gen++
	gen := e.gen
	e.bookingID, e.start = bookingID, start
	e.mode, e.loadErr, e.loadFailures = domain.AttendantLoading, "", 0
	e.packArrival, e.pack = "", nil
	e.detector = newPhaseDetector(phaseConfig{start: start, landing: domain.DefaultLandingThresholds})
	e.prepared, e.prefetch = map[string][]domain.SoundInstruction{}, nil
	e.queue, e.playing, e.lastPlayed, e.history = nil, nil, nil, nil
	e.repeatDue, e.musicOff, e.musicIdx, e.pending = map[int]time.Time{}, false, 0, 0
	e.retries = nil
	e.reasons, e.counts, e.current = map[string]domain.AttendantReason{}, map[string]int{}, ""
	e.jobs = make(chan attendantJob, attendantJobDepth)
	e.stop = make(chan struct{})
	jobs, stop := e.jobs, e.stop
	jobs <- attendantJob{gen: gen, kind: jobLoad}
	e.mu.Unlock()

	go e.run(gen, jobs, stop)
	e.notify()
}

// Stop ends the flight's announcements. The phase map and the history stay
// for the tab until the next flight starts. It does not block and emits
// nothing, since it is called with the flight lock held; the webview stops
// its player when the flight state goes idle.
func (e *attendant) Stop() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
}

// stopLocked ends the worker and the playback. Callers hold e.mu.
func (e *attendant) stopLocked() {
	if e.stop != nil {
		close(e.stop)
		e.stop = nil
	}
	e.gen++
	e.mode = domain.AttendantIdle
	e.queue = nil
	if e.playing != nil {
		e.recordLocked(*e.playing, domain.ClipStopped)
		e.playing = nil
	}
}

// Observe takes one reading of the aircraft. It is called from the data
// stream and, in the flare, from the high-rate sampler; it never blocks.
func (e *attendant) Observe(fd *domain.FlightData) {
	if e == nil {
		return
	}
	if fd == nil {
		return
	}
	gsx := e.deps.gsx()

	e.mu.Lock()
	e.gsxState, e.seatbelt = gsx, fd.Cabin
	if e.detector == nil || (e.mode != domain.AttendantLoading && e.mode != domain.AttendantClient) {
		e.mu.Unlock()
		return
	}
	now := e.deps.now()
	fires := e.detector.step(phaseSample{at: now, fd: fd, gsx: gsx})
	var cut string
	for _, f := range fires {
		if c := e.firedLocked(f, now); c != "" {
			cut = c
		}
	}
	e.repeatsLocked(now)
	e.retriesLocked(now)
	e.mu.Unlock()

	e.interrupt(cut)
	if len(fires) > 0 {
		e.notify()
	}
}

// firedLocked records a phase the detector fired and hands it to the
// worker. It returns the clip to cut short, if the phase ends boarding while
// its music plays. Callers hold e.mu.
func (e *attendant) firedLocked(f phaseFire, now time.Time) (cut string) {
	key := f.stepKey()
	e.reasons[key] = f.reason
	e.counts[key]++
	if f.phase != domain.PhaseSeatbeltOn && f.phase != domain.PhaseSeatbeltOff &&
		f.phase != domain.PhaseAlternate && !(f.phase == domain.PhaseCruise && f.cruiseAt > 0) {
		e.current = key
	}
	observability.Count("attendant.phase_fired", "phase", f.phase)
	slog.Info("attendant: phase", "attendant.phase", key, "attendant.reason", f.reason.Code)

	if f.phase == domain.PhaseBoardingComplete {
		cut = e.endBoardingLocked()
	}
	if !e.enabled {
		e.history = appendHistory(e.history, domain.AttendantHistoryEntry{
			At: now, Phase: f.phase, Service: f.service, Outcome: domain.ClipMuted, Reason: f.reason})
		return cut
	}
	e.sendLocked(attendantJob{gen: e.gen, kind: jobFire, fire: f, during: e.current})
	return cut
}

// sendLocked hands a job to the worker without waiting. Callers hold e.mu.
func (e *attendant) sendLocked(j attendantJob) {
	select {
	case e.jobs <- j:
		if j.kind == jobFire || j.kind == jobRepeat || j.kind == jobRetry {
			e.pending++
		}
	default:
		observability.Count("attendant.job_dropped")
		slog.Warn("attendant: worker is behind; dropping a phase", "attendant.phase", j.fire.phase)
	}
}

// ArrivalChanged is told the booking's new arrival in flight: a diversion.
// The pack is read again for the alternate, so lines name it, and the
// diversion is announced.
func (e *attendant) ArrivalChanged(icao string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.mode != domain.AttendantClient && e.mode != domain.AttendantLoading {
		e.mu.Unlock()
		return
	}
	e.sendLocked(attendantJob{gen: e.gen, kind: jobArrival, arrival: icao})
	e.mu.Unlock()
}

// run is the flight's worker: it does the network work jobs ask for, in
// order, and downloads the pack's audio while there is nothing else to do.
func (e *attendant) run(gen int, jobs <-chan attendantJob, stop <-chan struct{}) {
	defer observability.Recover()

	var retry <-chan time.Time
	do := func(j attendantJob) {
		if e.handle(j) {
			retry = time.After(packRetryEvery)
		}
	}
	for {
		// Jobs first; the audio of the rest of the pack only while there
		// are none.
		select {
		case <-stop:
			return
		case j := <-jobs:
			do(j)
			continue
		default:
		}
		if e.prefetchOne(gen) {
			continue
		}
		select {
		case <-stop:
			return
		case j := <-jobs:
			do(j)
		case <-retry:
			retry = nil
			do(attendantJob{gen: gen, kind: jobLoad})
		}
	}
}

// handle does one job and reports whether the pack must be read again later.
func (e *attendant) handle(j attendantJob) (retry bool) {
	if !e.isGen(j.gen) {
		return false
	}
	switch j.kind {
	case jobLoad:
		return e.load(j.gen)
	case jobFire:
		e.play(j.gen, j.fire, j.during)
		e.settled(j.gen)
	case jobRepeat:
		e.repeat(j.gen, j.line)
		e.settled(j.gen)
	case jobRetry:
		e.retryLine(j.gen, j.retry)
		e.settled(j.gen)
	case jobArrival:
		e.divert(j.gen, j.arrival)
	}
	return false
}

// settled counts an announcement job done, and lets the boarding music
// resume if that was the last one.
func (e *attendant) settled(gen int) {
	e.mu.Lock()
	idle := false
	if e.gen == gen && e.pending > 0 {
		e.pending--
		idle = e.pending == 0 && e.playing == nil && len(e.queue) == 0 && e.boardingLocked()
	}
	e.mu.Unlock()
	if idle {
		e.clipReady()
	}
}

// isGen reports whether gen is still the flight under way.
func (e *attendant) isGen(gen int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gen == gen
}

// lineKey identifies a line's audio within a pack revision.
func lineKey(l speechLine) string {
	return fmt.Sprintf("%d:%d", l.id(), l.service())
}

// notify sends the tab a fresh snapshot.
func (e *attendant) notify() {
	if e.deps.emit == nil {
		return
	}
	snap := e.Snapshot()
	e.deps.emit(attendantStateEvent, &snap)
}

// interrupt tells the player to cut a clip short.
func (e *attendant) interrupt(id string) {
	if id == "" || e.deps.emit == nil {
		return
	}
	e.deps.emit(attendantInterruptEvent, id)
}

func (e *attendant) clipReady() {
	if e.deps.emit != nil {
		e.deps.emit(attendantClipReadyEvent, true)
	}
}

func appendHistory(h []domain.AttendantHistoryEntry, entry domain.AttendantHistoryEntry) []domain.AttendantHistoryEntry {
	h = append(h, entry)
	if len(h) > attendantHistoryMax {
		h = h[len(h)-attendantHistoryMax:]
	}
	return h
}
