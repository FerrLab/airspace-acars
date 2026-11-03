# Flight attendant: the ACARS plays the cabin announcements

Status: proposed (beta). Airspace side: FerrLab/airspace, branch
`claude/trusting-lovelace-z76iay`.

## Goal

Until now Airspace decided *when* a cabin announcement played: it detected the
flight phase from position reports, rendered the speech pack into one MP3 per
phase and queued it, and the ACARS polled `GET /api/v2/acars/sound` every ten
seconds and played whatever came back. That is late (position reports are
batched), blind to the cabin (seatbelt sign, boarding) and cannot do anything
that depends on the pilot's machine, like GSX.

The new speech pack editor in Airspace adds triggers that only the ACARS can
see — boarding started and finished (GSX), the seatbelt sign switched on or off
in flight, meal services, lines that depend on the touchdown — so the ACARS
now downloads the pack and plays the right line at the right moment itself.

## Backwards compatibility

- An ACARS that predates this keeps working exactly as before: the server
  still detects phases and queues audio for `GET /sound`.
- This ACARS tells the server it plays the announcements itself by sending
  `"speech": "client"` with `POST /start`, and again implicitly by reading
  `GET /speech/pack`. From then on the server stops queuing audio for that
  booking, so nothing plays twice.
- Against an Airspace that predates this (`GET /speech/pack` answers 404 for
  a pilot who *has* a booking, or 405/404 because the route is missing), the
  ACARS falls back to the old polling player.

## API (Airspace, pilot token, `/api/v2/acars`)

All three answer `404 {"message": "No active booking"}` when the pilot has no
booking, like `/sound`.

### `GET /speech/pack[?arrival=ICAO]`

The whole resolved pack for the active booking. `arrival` asks for the pack as
it would be at another arrival airport (packs can be assigned to arrival
airports, and lines name the destination); without it the booking's current
arrival is used — after a diversion that is already the alternate.

```json
{
  "data": {
    "version": 1,
    "revision": "9f2c…",              // changes whenever anything below changes
    "booking_id": 123,
    "flight": {
      "departure": "LPPT",
      "arrival": "SBGR",
      "is_alternate": false,
      "original_arrival": null,         // ICAO of the planned destination after a diversion
      "planned_block_minutes": 590,
      "time_of_day": "night",           // "day" | "night", at departure, local time
      "meal_services": [
        { "n": 1, "meal": "dinner",    "timing": "after_cruise",   "minutes": 30 },
        { "n": 2, "meal": "breakfast", "timing": "before_descent", "minutes": 90 }
      ]
    },
    "settings": {
      "seatbelt": { "on_cooldown_minutes": 2, "off_cooldown_minutes": 2 },
      "landing":  { "smooth_max_fpm": 200, "smooth_max_g": 1.2, "hard_min_fpm": 400, "hard_min_g": 1.5 },
      "night":    { "from": "22:00", "to": "06:00" }
    },
    "phases": [
      { "phase": "gate_departure", "label": "Boarding", "lines": [ LINE, … ] },
      …every phase in flight order, including ones with no lines…
    ]
  }
}
```

`LINE`:

```json
{
  "id": 42,                    // speech template id; stable while the pack is unchanged
  "phase": "gate_departure",
  "sort_order": 0,             // play order inside the phase
  "type": "text",              // "text" (TTS) | "audio" (uploaded file)
  "language": "en",            // null for audio
  "voice": "Lily",             // display only; null for audio or no voice
  "conditions": {
    "time_of_day": "any",      // any | day | night  (already applied: see playable)
    "landing": "any",          // any | smooth | hard   (after_landing only)
    "cruise_at": "cruise",     // cruise | 25 | 50 | 75 (cruise only; % of planned block time)
    "meal": "any",             // any | breakfast | lunch | dinner | snack (meal_service only)
    "repeat": false,           // gate_departure only: replay every repeat_every_minutes until boarding_complete
    "repeat_every_minutes": 5
  },
  "service": null,             // meal_service lines are listed once per meal service they play at: 1 | 2
  "text": "Good evening, ladies and gentlemen…",   // rendered; null for audio
  "playable": true,
  "skip_reason": null,         // when not playable: "disabled" | "time_of_day" | "meal_not_served" | "no_voice" | "empty"
  "dynamic": false,            // text uses a time-dependent variable (local_time, arr_status, delay, salute…): fetch again just before playing
  "audio_url": "https://tenant/api/v2/acars/sound/combined/<sha256>.mp3",  // one file for the line; null until generated
  "segments": [ {"type": "play", "url": "…"}, {"type": "pause", "duration_ms": 300} ],  // same line as play/pause steps, for when audio_url is null but the audio exists (server without ffmpeg); [] until generated
  "fetch_url": "https://tenant/api/v2/acars/speech/lines/42?service=1",
  "duration_ms": 5400          // null when unknown
}
```

Reading the manifest also starts generating every playable line's audio in the
background, so `fetch_url` is usually instant by the time it is needed.

### `GET /speech/lines/{id}[?arrival=ICAO&service=N]`

One line, rendered *now* (time-dependent variables are current) with its audio
generated before replying: `{"data": LINE}` with `audio_url` set.
`422` when it cannot be voiced (no voice for the language, empty text).

### `GET /speech/phases/{phase}[?arrival=&service=&landing=&cruise_at=]`

The playable lines of one phase for the given situation, each with audio:
`{"data": {"phase": "after_landing", "lines": [LINE…]}}`.
`landing` is `smooth | normal | hard`; a normal landing plays only lines set
to *any landing*. This is how a client fetches "only the landing for this
booking" once it knows the touchdown.

Audio URLs are the existing public proxies (`/sound/combined/*`,
`/sound/upload/*`) and are immutable: cache them by URL.

## Phases and when the ACARS fires them

Phase ids are the server's `SpeechTriggerPhase` values. `gate_departure` is
labelled *Boarding* in the editor; the id is kept for compatibility.

| Phase | Fires |
|---|---|
| `gate_departure` (Boarding) | GSX boarding becomes active; without GSX, when the flight starts on the ground. Lines with `repeat` replay every N minutes until boarding completes; uploaded audio with `repeat` loops between them (boarding music). |
| `boarding_complete` | GSX boarding completed; without GSX, right before pushback (or taxi), as the server plays it for older clients. |
| `pushback` | GSX pushback active, or the aircraft moving backwards on the ground. |
| `taxi` | Forward ground movement with an engine running, after pushback. |
| `climb_out` | 60 s after lift-off, climbing. |
| `passing_10000` | Climbing through 10,000 ft. |
| `seatbelt_off` / `seatbelt_on` | The seatbelt sign changes while airborne, not again within the pack's cooldown. |
| `cruise` | Level at cruise for the configured time; `cruise_at` lines at 25/50/75 % of the planned block time. |
| `meal_service` | Service 1: N minutes after reaching cruise. Service 2: N minutes before the estimated top of descent. Each service plays its *any* lines, then its meal's lines. |
| `descend` | Sustained descent after cruise. |
| `descending_10000` | Descending through 10,000 ft. |
| `final_approach` | Gear down below 2,000 ft AGL, descending, near the arrival. |
| `after_landing` | Rollout finished after touchdown; the touchdown's rate and g pick smooth / normal / hard. |
| `gate_arrival` | Parked at the gate: on the ground, stopped, engines off (or GSX deboarding). |
| `alternate_declared` | The booking's arrival changed in flight (diversion). The manifest is re-read for the new arrival before later phases. |

Each phase fires once per flight (seatbelt phases excepted). Lines of a phase
play back to back; phases queue and never cut each other off.

### As implemented in the ACARS

The ACARS side refines the table above; [../flight-attendant.md](../flight-attendant.md)
has the full detail.

- **With GSX**, boarding waits for GSX boarding *active* (state 5), not
  requested (4); boarding complete is GSX boarding completed or the beacon
  coming on; pushback is GSX departure active or moving over 1 kt with the
  engines off; taxi is over 5 kt with an engine running.
- **Without GSX the old triggers are kept**, the ones the server used before
  the ACARS timed the announcements: boarding plays when the flight starts on
  the ground (whatever the beacon or engines are doing); pushback is the
  aircraft leaving "Parked" (stopped, engines off) by moving at 1–5 kt or by
  an engine starting while stopped; taxi is the first 5 kt; boarding complete
  plays right before whichever of pushback or taxi comes first, as the server
  plays it for older clients. A flight never parked has no pushback.
- **climb_out**: 60 s after lift-off *and* above 1,000 ft AGL, climbing.
- **cruise**: within 300 fpm of level above 10,000 ft, held **two minutes**.
  `cruise_at` lines wait while below 10,000 ft or sinking over 500 fpm and are
  dropped once the descent starts.
- **meal_service** `before_descent` uses top of descent = flight start +
  `planned_block_minutes` − 30 min; a service not yet played when the descent
  starts is dropped.
- **descend**: sinking over 500 fpm for 10 s and 500 ft below the cruise level.
- **final_approach** does not check the distance to the arrival.
- **after_landing**: rate = last reading in the air (the flare is sampled at
  30 Hz), load = the largest in the 2 s after touchdown; announced once the
  rollout is under 15 kt.
- **gate_arrival** also fires on GSX deboarding requested.
- `fetch_url` is not followed: the ACARS builds `/speech/lines/{id}` itself so
  the pilot token only goes to `/api/v2/acars/` on the tenant. A line is
  fetched again before it plays when `dynamic` is set or it has neither
  `audio_url` nor `segments`; a 404 for it re-reads the manifest.

## Out of scope

- Server-side detection of the new ACARS-only triggers for old clients
  (seatbelt and GSX boarding need the pilot's machine). Old clients still get
  `boarding_complete` just before pushback and meal service 1 in cruise.
- Per-pilot voice choice.
