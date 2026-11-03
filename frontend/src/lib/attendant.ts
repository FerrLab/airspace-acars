// The Flight Attendant tab's view of AttendantService.GetSnapshot, typed here
// as debug-snapshot.ts types the debug snapshot, so tests need no generated
// bindings. Field names follow internal/domain/attendant.go.

export type AttendantMode = "idle" | "loading" | "client" | "legacy" | "no_booking";
export type StepStatus = "pending" | "fired" | "skipped";

export interface SoundSegment {
  type: string;
  localFile?: string;
  duration_ms: number;
}

export interface AttendantReason {
  code: string;
  altitude?: number;
  vs?: number;
  gs?: number;
  g?: number;
  seconds?: number;
  minutes?: number;
  percent?: number;
  service?: number;
  landing?: string;
  airport?: string;
}

export interface AttendantStep {
  key: string;
  phase: string;
  service?: number;
  status: StepStatus;
  lines: number;
  firedAt: string;
  count: number;
  reason: AttendantReason;
}

export interface AttendantClip {
  id: string;
  lineId: number;
  phase: string;
  service?: number;
  text: string;
  language: string;
  voice: string;
  music: boolean;
  reason: AttendantReason;
  segments: SoundSegment[] | null;
}

export interface AttendantHistoryEntry {
  at: string;
  phase: string;
  service?: number;
  text: string;
  language: string;
  voice: string;
  music: boolean;
  outcome: "played" | "failed" | "skipped" | "stopped" | "interrupted" | "muted";
  reason: AttendantReason;
}

export interface GSXService {
  state: number;
  completed: boolean;
  progress: string;
}

export interface GSXState {
  connected: boolean;
  running: boolean;
  boarding: GSXService;
  deboarding: GSXService;
  departure: GSXService;
}

export interface AttendantSnapshot {
  enabled: boolean;
  mode: AttendantMode;
  current: string;
  arrival: string;
  steps: AttendantStep[] | null;
  nowPlaying: AttendantClip | null;
  queue: AttendantClip[] | null;
  history: AttendantHistoryEntry[] | null;
  gsx: GSXState;
  seatbelt: { seatbeltSign: boolean; seatbeltKnown: boolean };
  seatbelts: { on: number; off: number };
  error: string;
}

const GO_ZERO_TIME = "0001-01-01T00:00:00Z";
const idleService: GSXService = { state: 0, completed: false, progress: "" };

export const EMPTY_ATTENDANT: AttendantSnapshot = {
  enabled: true,
  mode: "idle",
  current: "",
  arrival: "",
  steps: [],
  nowPlaying: null,
  queue: [],
  history: [],
  gsx: { connected: false, running: false, boarding: idleService, deboarding: idleService, departure: idleService },
  seatbelt: { seatbeltSign: false, seatbeltKnown: false },
  seatbelts: { on: 0, off: 0 },
  error: "",
};

// --- the phase map ---------------------------------------------------------

/** The flight profile line, as the Airspace speech pack editor draws it. */
export const PROFILE_POINTS = "25,128 245,128 340,88 420,40 665,40 760,88 850,118 890,128 975,128";
/** The diversion branch, dashed. */
export const DIVERSION_POINTS = "850,118 960,62";
export const MAP_VIEWBOX = "0 0 1000 172";

export interface MapMarker {
  key: string;
  x: number;
  y: number;
}

/** Where each phase sits on the profile, in flight order. */
export const MAP_MARKERS: MapMarker[] = [
  { key: "gate_departure", x: 25, y: 128 },
  { key: "boarding_complete", x: 85, y: 128 },
  { key: "pushback", x: 145, y: 128 },
  { key: "taxi", x: 205, y: 128 },
  { key: "climb_out", x: 285, y: 111 },
  { key: "passing_10000", x: 340, y: 88 },
  { key: "cruise", x: 440, y: 40 },
  { key: "meal_service:1", x: 515, y: 40 },
  { key: "meal_service:2", x: 600, y: 40 },
  { key: "descend", x: 680, y: 48 },
  { key: "descending_10000", x: 760, y: 88 },
  { key: "final_approach", x: 850, y: 118 },
  { key: "after_landing", x: 915, y: 128 },
  { key: "gate_arrival", x: 975, y: 128 },
  { key: "alternate_declared", x: 960, y: 62 },
];

/** The phases shown before a flight has a pack: the map's own list. */
const DEFAULT_KEYS = [...MAP_MARKERS.map((m) => m.key).slice(0, 9), "seatbelt_off", "seatbelt_on",
  ...MAP_MARKERS.map((m) => m.key).slice(9)];

export type MarkerState = "fired" | "playing" | "pending" | "silent" | "skipped";

/** The phase map key a clip belongs to. */
export function clipKey(clip: Pick<AttendantClip, "phase" | "service"> | null | undefined): string {
  if (!clip) return "";
  if (clip.phase === "meal_service" && clip.service) return `meal_service:${clip.service}`;
  return clip.phase;
}

/** The snapshot's steps, or every phase pending when there is no flight yet. */
export function stepsOf(snap: AttendantSnapshot): AttendantStep[] {
  if (snap.steps && snap.steps.length > 0) return snap.steps;
  return DEFAULT_KEYS.map((key) => {
    const [phase, service] = key.split(":");
    return {
      key, phase, service: service ? Number(service) : undefined, status: "pending" as const,
      lines: -1, firedAt: GO_ZERO_TIME, count: 0, reason: { code: "" },
    };
  });
}

/**
 * How a marker is drawn: playing wins, then fired; a phase the pack has no
 * lines for is silent whatever happened to it.
 */
export function markerState(step: AttendantStep | undefined, playingKey: string): MarkerState {
  if (!step) return "pending";
  if (step.key === playingKey) return "playing";
  if (step.lines === 0) return "silent";
  if (step.status === "fired") return "fired";
  if (step.status === "skipped") return "skipped";
  return "pending";
}

/** Where the aircraft icon sits: the last phase fired, or the gate. */
export function aircraftMarker(snap: AttendantSnapshot): MapMarker | null {
  if (snap.mode === "idle" && !snap.current) return null;
  const key = snap.current || "gate_departure";
  return MAP_MARKERS.find((m) => m.key === key) ?? null;
}

// --- words -----------------------------------------------------------------

/** The translation key for a step's name. */
export function stepLabelKey(key: string): string {
  return `attendant.phase.${key.replace(":", "_")}`;
}

/** "en · Lily", or what is known of it. */
export function languageVoice(clip: Pick<AttendantClip, "language" | "voice">): string {
  return [clip.language?.toUpperCase(), clip.voice].filter(Boolean).join(" · ");
}

type Translate = (key: string, options?: Record<string, unknown>) => string;

/** Why a clip played, in the pilot's language. */
export function reasonText(t: Translate, reason: AttendantReason | null | undefined): string {
  if (!reason?.code) return "";
  const values: Record<string, unknown> = { ...reason };
  if (typeof reason.altitude === "number") values.altitude = reason.altitude.toLocaleString("en-US");
  if (typeof reason.g === "number") values.g = reason.g.toFixed(2);
  if (typeof reason.vs === "number") values.vs = formatSigned(reason.vs);
  if (reason.landing) values.landing = t(`attendant.landing.${reason.landing}`);
  return t(`attendant.reason.${reason.code}`, values);
}

/** −142 with a real minus sign, +1,500 with a plus. */
export function formatSigned(n: number): string {
  const abs = Math.abs(n).toLocaleString("en-US");
  if (n < 0) return `−${abs}`;
  if (n > 0) return `+${abs}`;
  return abs;
}

/** HH:MM in the pilot's clock, or "" for an unset Go time. */
export function clockTime(iso: string | null | undefined): string {
  if (!iso || iso.startsWith("0001-01-01")) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** The translation key for a GSX service state (L:FSDT_GSX_*_STATE values). */
export function gsxStateKey(state: number): string {
  const names: Record<number, string> = {
    1: "callable", 2: "notAvailable", 3: "bypassed", 4: "requested", 5: "active", 6: "completed", 7: "completing",
  };
  return `attendant.gsx.state.${names[state] ?? "unknown"}`;
}

/** Whether the old GET /sound poll should run: only against an old server. */
export function legacyPollActive(snap: Pick<AttendantSnapshot, "mode" | "enabled">): boolean {
  return snap.mode === "legacy" && snap.enabled;
}
