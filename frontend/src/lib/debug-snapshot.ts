// The debug screen's view of DebugService.GetDebugSnapshot, typed here as
// use-flight-data.ts types FlightData, so tests need no generated bindings.

export type ReportOutcome = "" | "sent" | "outbox" | "dropped" | "preview";

export interface GroundStatus {
  airport: string;
  runways: number;
  stands: number;
  runway: string;
  stand: string;
  loadedAt: string;
  lastError: string;
}

export interface PositionReportSnapshot {
  json: string;
  at: string;
  outcome: ReportOutcome;
  batchSize: number;
}

export interface DebugSnapshot {
  ground: GroundStatus;
  report: PositionReportSnapshot;
  profiles: string[];
}

const GO_ZERO_TIME = "0001-01-01T00:00:00Z";

export const EMPTY_SNAPSHOT: DebugSnapshot = {
  ground: { airport: "", runways: 0, stands: 0, runway: "", stand: "", loadedAt: GO_ZERO_TIME, lastError: "" },
  report: { json: "", at: GO_ZERO_TIME, outcome: "", batchSize: 0 },
  profiles: [],
};

/** Go encodes an unset time.Time as year 1. */
export function isUnsetTime(iso: string | null | undefined): boolean {
  return !iso || iso.startsWith("0001-01-01");
}

/** Whole seconds from iso to now, or null when the time is unset or unreadable. */
export function secondsSince(iso: string | null | undefined, now: number): number | null {
  if (isUnsetTime(iso)) return null;
  const t = Date.parse(iso as string);
  if (Number.isNaN(t)) return null;
  return Math.max(0, Math.round((now - t) / 1000));
}

const OUTCOME_KEYS: Record<string, string> = {
  sent: "debug.outcome.sent",
  outbox: "debug.outcome.outbox",
  dropped: "debug.outcome.dropped",
  preview: "debug.outcome.preview",
};

/** The translation key for what became of a report. */
export function outcomeKey(outcome: string): string {
  return OUTCOME_KEYS[outcome] ?? "debug.outcome.none";
}
