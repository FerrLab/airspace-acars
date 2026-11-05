# Flight attendant

The ACARS plays the airline's cabin announcements itself. When a flight
starts it reads the booking's speech pack from Airspace, follows the flight
from the simulator, GSX and the booking, and plays each phase's lines when the
aircraft gets there. The contract with the server is in
[plans/2026-11-03-flight-attendant-design.md](plans/2026-11-03-flight-attendant-design.md).

## Modes

| Mode | When | What plays |
|---|---|---|
| `client` | `GET /speech/pack` answered with a pack | This ACARS detects the phases and plays the pack |
| `legacy` | The route is missing (404 without "No active booking", or 405) | The old `GET /sound` poll, every ten seconds, as before |
| `no_booking` | 404 "No active booking" | Nothing |
| `loading` | Before the pack is in: while the server prepares it (202, polled every `retry_after`), and while it is retried after a failure (every 30 s) | Nothing yet; phases that fire wait for the pack |
| `idle` | No flight | Nothing |

`POST /start` carries `"speech": "client"`, so a server that knows the field
stops queuing `/sound` audio for the booking; an older server ignores it and
the ACARS falls back to the poll. In client mode the poll is not made at all.

The **Announcements** switch on the Flight Attendant tab (setting
`flightAttendant`, on by default) turns everything off. Phases are still
followed while it is off, so turning it back on carries on from where the
flight is; what fired meanwhile shows as *muted* in the history.

## When each phase fires

Detection is `internal/app/attendant_phases.go` (with `attendant_cruise.go`
and `attendant_landing.go`): a pure state machine fed one reading at a time
with its time, at once a second from the data stream and thirty times a second
in the flare. The thresholds follow the server's old detector and MSFS
Universal Announcer (manual v0.9.9.7), with the deviations noted.

| Phase | Fires when |
|---|---|
| Boarding (`gate_departure`) | GSX boarding is *active* (state 5; requested is not enough). Without GSX: when the flight starts on the ground, whatever the beacon or engines are doing (the old trigger). |
| `boarding_complete` | With GSX: GSX boarding completed (state 6, or dropped from in progress back to callable), or the beacon switching on, whichever is first. Without GSX: right before pushback or taxi, whichever comes first (the old trigger). Ends the boarding repeats and music. |
| `pushback` | With GSX: GSX departure active, or moving over 1 kt with the engines off. Without GSX: leaving "Parked" (stopped, engines off) by moving at 1–5 kt or by starting an engine while stopped (the old trigger); a flight never parked has none. |
| `taxi` | With GSX: on the ground, an engine running, over 5 kt. Without GSX: the first 5 kt (the old trigger). Without a pushback, pushback is skipped. |
| `climb_out` | 60 s after lift-off, above 1,000 ft AGL and climbing. Universal Announcer waits for 3,000 ft and two minutes, late for a short sector. |
| `passing_10000` | Climbing through 10,000 ft MSL. |
| `cruise` | Within 300 fpm of level above 10,000 ft for **two minutes**. The server used ten minutes; one minute is an ordinary level-off on the climb. |
| `cruise` at 25/50/75 % | At that share of the planned block time from flight start, once cruising; held while below 10,000 ft or sinking over 500 fpm, dropped once the descent begins. |
| `meal_service` | `after_cruise`: N minutes after reaching cruise. `before_descent`: N minutes before the estimated top of descent, taken as flight start + planned block − 30 min. Dropped once the descent begins. |
| `seatbelt_on` / `seatbelt_off` | In flight, after the climb-out, when the aircraft's seatbelt sign changes and holds the new state for 2 s, and not within the pack's cooldown for that announcement. A sign switched and back within the 2 s is no change; one toggled a few times is announced once, for where it ends up. Only aircraft that report the sign (see below). |
| `descend` | After cruise, sinking over 500 fpm for 10 s and 500 ft below the cruise level (1,000 ft below the highest point when no cruise was held). |
| `descending_10000` | Descending through 10,000 ft MSL after cruise or descent. |
| `final_approach` | Gear down below 2,000 ft AGL, sinking over 300 fpm. Universal Announcer uses 3,000 ft. |
| `after_landing` | The rollout has slowed below 15 kt. The touchdown's rate is the last reading in the air, its load the largest in the two seconds after; a bounce is the same landing. Classified with the pack's `settings.landing` and fetched with `GET /speech/phases/after_landing?landing=`. |
| `gate_arrival` | After landing, engines off and stopped, or GSX deboarding requested or active. |
| `alternate_declared` | The booking's arrival changed in flight. The pack is read again with `?arrival=`, the audio prepared for the old destination is dropped, and the diversion is announced. |

Each phase fires once per flight except the seatbelt announcements and a
diversion. Phases queue and never cut each other off.

## Loading the pack

`GET /speech/pack` may answer **202 Accepted** while the server is still
voicing the booking's lines:
`{"data":{"status":"preparing","lines_ready":12,"lines_total":28,"retry_after":10}}`.
The server answers 200 with the pack once every playable non-dynamic line has
its audio, or 120 s after the booking's first request, whichever comes first;
a request with `?arrival=` (a diversion) never gets a 202. The ACARS
(`fetchSpeechPack` and `load` in `internal/app/attendant_pack.go` and
`attendant_load.go`):

- asks again after `data.retry_after` seconds, 10 s when it is missing,
  clamped to 2–60 s. The server's `Retry-After` header carries the same
  number, but `DoRequest` returns only the body;
- shows *Preparing announcements (12/28)…* on the tab instead of an error,
  logs at `Debug` and counts no failure;
- keeps the phases that fire meanwhile (boarding fires on the first reading)
  and plays them once the pack is in, if the flight is still at the step they
  fired at. One the flight has moved past is dropped.

Any other failure (the network, a 5xx, a body that is not a pack) is retried
every 30 s and shown as an error, as before.

## Playback

The engine (`internal/app/attendant.go`) owns the queue; the webview owns the
audio output, device and volume. When a clip is ready the backend emits
`attendant-clip-ready`; the player in the app shell
(`hooks/use-attendant-player.ts`) takes clips with `AttendantService.TakeClip`
until there are none, plays each clip's segments in order (one combined file,
or the line's `segments` when the server could not combine them), and reports
`ClipFinished` or `ClipFailed`. `attendant-interrupt` cuts a clip short.

- Clips are 1.5 s apart (`CLIP_GAP_MS` in `lib/clip-player.ts`): the player
  waits that long after a clip ends, however it ended, before it takes the
  next, so a phase's lines and phases that fire together do not run into each
  other. The waiting clip stays in the backend's queue, where Skip and Stop all
  reach it. The segments inside a clip get no gap: they are one line.
- Boarding lines with `repeat` play again `repeat_every_minutes` after they
  last **finished**. Uploaded audio with `repeat` is boarding music: it loops
  whenever nothing else is waiting, and an announcement cuts it short.
- Lines marked `dynamic`, or whose audio is not generated yet, are fetched again
  with `GET /speech/lines/{id}` just before they play; the rest of the pack is
  downloaded in the background, in flight order, as soon as it is read. Audio
  is cached by URL in the system temp folder.
- A line the server no longer has (404) makes the ACARS read the pack again.
- A line the server could not voice yet (422 with reason `not_generated`:
  its text-to-speech provider failed or timed out), or lost to the network or
  a 5xx, is asked for again after 30 s, 1 min and 2 min, while the flight is
  still at the phase that fired it (`internal/app/attendant_retry.go`). It
  plays for that phase when it comes. One the flight has moved past, or still
  not ready after the third retry, shows as failed. Any other 422 reason
  (`no_voice`, `empty`, …) is final.

## Seatbelt sign

`FlightData.Cabin` carries the sign. The stock route reads `CABIN SEATBELTS
ALERT SWITCH` on MSFS and `sim/cockpit2/switches/fasten_seat_belts` on X-Plane
(AUTO is lit below 10,000 ft). Profiles bind the aircraft's own switch where it
has one: Fenix `S_OH_SIGNS`, the Zibo `seatbelt_sign_pos`, and an unverified
PMDG 737 `switch_104_73X`; FBW and the PMDG 777 keep the stock variable. Each
profile's `notes` say where its variable comes from.

## GSX

Without GSX the boarding, boarding complete, pushback and taxi triggers are
the ones the server used before the ACARS timed the announcements (see the
phase table), so a pilot who does not run GSX hears them when they always
did. GSX is "running" when its remote control server answers; the choice is
made reading by reading, so starting GSX mid-turnaround switches to it.

`internal/adapters/gsx` connects to GSX Pro's remote control server at
`ws://127.0.0.1:8744` (GSX 4.0.6 or later, "Remote control server" enabled,
which is the default), subscribes to its state channel, and follows the
Boarding, Departure (pushback) and Deboarding services. It retries every five
seconds while GSX is not running and logs that at Debug only. A state
snapshot is over a megabyte once an airport is loaded (1.2 MB at SBBR, almost
all of it `handlerData`), so one message may be up to 16 MiB.

The protocol comes from the third-party client Fragtality/Any2GSX; the official
guide ships only inside the GSX installer. `stateRaw` is assumed to carry the
values of `L:FSDT_GSX_<SERVICE>_STATE` (1 callable … 5 active, 6 completed),
which is unverified. A server that asks for authentication is reported once
and not used.

## The Flight Attendant tab

The tab draws the flight profile as the Airspace speech pack editor does, with
a marker per phase (fired, playing, pending, skipped, or silent when the pack
has no lines for it), the aircraft at the current phase, and the seatbelt sign
announcements with their counts. Below it: what is playing, in which language
and voice and why ("Touchdown −142 fpm · 1.08 g → smooth landing"); the phases
with their lines, time and reason, and a button to play one now for trying a
pack; the history; GSX and the seatbelt sign. Skip, Replay and Stop all act on
the queue; Stop all also ends the boarding music and repeats.

## Not verified

The detection is covered by unit tests on scripted flights. It has not been
flown in MSFS or X-Plane, against a real GSX, or with every profile's seatbelt
variable; the PMDG and Zibo values in particular are unverified.
