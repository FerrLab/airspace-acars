/**
 * Pure helpers for the flight detail tabs. Everything here reads values the
 * platform recorded for the flight; nothing fills in a missing value, so a
 * caller that gets null hides the row instead of showing a guess.
 */
import type {
  FlightDetail,
  LandingReport,
  RunwayWind,
  TakeoffReport,
} from "../../bindings/airspace-acars/internal/domain/models";

export type DetailTab = "overview" | "landing" | "takeoff" | "telemetry" | "score";

/**
 * The tabs worth showing for this flight. Without a detail (the airline's
 * server predates the endpoint, or the request failed) only Overview shows,
 * since the track would fail the same way. Otherwise Telemetry always shows
 * and says so when there is no track; Landing and Takeoff show only when the
 * platform recorded that section, and Score once the breakdown has a category
 * to explain.
 */
export function availableTabs(detail: FlightDetail | null): DetailTab[] {
  const tabs: DetailTab[] = ["overview"];
  if (!detail) return tabs;
  if (detail.landing) tabs.push("landing");
  if (detail.takeoff) tabs.push("takeoff");
  tabs.push("telemetry");
  if ((detail.score_breakdown?.categories?.length ?? 0) > 0) tabs.push("score");
  return tabs;
}

/** A translation key with its interpolation values. */
export interface Phrase {
  key: string;
  values?: Record<string, string | number>;
}

/**
 * Where on the runway width a touchdown or liftoff happened. Offsets are
 * metres right (+) or left (-) of the centreline; under 0.05 m reads as on
 * the centreline because the platform rounds to centimetres.
 */
export function centrelineOffset(offsetM: number | null | undefined): Phrase | null {
  if (offsetM == null) return null;
  const metres = Math.abs(offsetM);
  if (metres < 0.05) return { key: "myFlights.runway.onCentreline" };
  return {
    key: offsetM > 0 ? "myFlights.runway.rightOfCentreline" : "myFlights.runway.leftOfCentreline",
    values: { value: metres.toFixed(1) },
  };
}

/** A touchdown distance, which is negative when short of the threshold. */
export function touchdownDistance(distanceM: number | null | undefined): Phrase | null {
  if (distanceM == null) return null;
  if (distanceM < 0) return { key: "myFlights.landing.shortOfThreshold", values: { value: Math.abs(distanceM) } };
  return { key: "myFlights.units.metres", values: { value: distanceM } };
}

/** The wind against the runway, as headwind/tailwind and crosswind phrases. */
export function windComponents(wind: RunwayWind | null | undefined): { along: Phrase; across: Phrase } | null {
  if (!wind) return null;
  const along: Phrase = {
    key: wind.headwind_knots >= 0 ? "myFlights.wind.headwind" : "myFlights.wind.tailwind",
    values: { value: Math.abs(wind.headwind_knots).toFixed(1) },
  };
  const across: Phrase =
    Math.abs(wind.crosswind_knots) < 0.05
      ? { key: "myFlights.wind.noCrosswind" }
      : {
          key: wind.crosswind_knots > 0 ? "myFlights.wind.crosswindRight" : "myFlights.wind.crosswindLeft",
          values: { value: Math.abs(wind.crosswind_knots).toFixed(1) },
        };
  return { along, across };
}

/**
 * Positions along a runway drawn left (threshold) to right, as percentages of
 * the drawn length. The drawn length stretches to fit a mark past the runway
 * end, so a long landing is never clipped into looking like it stopped in
 * time.
 */
export interface RunwayStrip {
  lengthM: number;
  /** Start of the coloured segment: touchdown, or the start of the roll. */
  fromPct: number;
  /** End of the coloured segment: the stop point, or liftoff. */
  toPct: number;
  /** The marked point: touchdown or liftoff. */
  markPct: number;
  /** Where the declared runway ends, below 100 when a mark overran it. */
  runwayEndPct: number;
}

function strip(runwayLengthM: number | null | undefined, from: number, to: number, mark: number): RunwayStrip | null {
  if (!runwayLengthM || runwayLengthM <= 0) return null;
  const drawn = Math.max(runwayLengthM, to, mark, from);
  const pct = (m: number) => Math.min(100, Math.max(0, (m / drawn) * 100));
  return { lengthM: runwayLengthM, fromPct: pct(from), toPct: pct(to), markPct: pct(mark), runwayEndPct: pct(runwayLengthM) };
}

/**
 * The landing strip from touchdown to the end of the rollout. Null when the
 * runway length or the touchdown point is unknown.
 */
export function landingStrip(landing: LandingReport): RunwayStrip | null {
  const touchdown = landing.touchdown_distance_m;
  if (touchdown == null) return null;
  const stop = landing.runway_used_m ?? touchdown;
  return strip(landing.runway?.length_m, Math.max(0, touchdown), Math.max(touchdown, stop), Math.max(0, touchdown));
}

/**
 * The takeoff strip from where the roll started to liftoff. Null when the
 * runway length or the liftoff point is unknown.
 */
export function takeoffStrip(takeoff: TakeoffReport): RunwayStrip | null {
  const liftoff = takeoff.liftoff_distance_m;
  if (liftoff == null) return null;
  const rollStart = takeoff.takeoff_roll_m != null ? Math.max(0, liftoff - takeoff.takeoff_roll_m) : liftoff;
  return strip(takeoff.runway?.length_m, rollStart, Math.max(0, liftoff), Math.max(0, liftoff));
}

/**
 * Text colour for a score on the platform's 0–10 scale, matching the web
 * flight page's thresholds.
 */
export function scoreTone(score: number | null | undefined): string {
  if (score == null) return "text-muted-foreground";
  if (score >= 8) return "text-emerald-400";
  if (score >= 6) return "text-lime-400";
  if (score >= 4) return "text-amber-400";
  return "text-red-400";
}

/** Text colour for an FDM event type. */
export function eventTone(type: string): string {
  if (type === "violation") return "text-red-400";
  if (type === "warning") return "text-amber-400";
  return "text-sky-400";
}

/** The translation key for an FDM event type, falling back to information. */
export function eventTypeKey(type: string): string {
  return type === "violation" || type === "warning" ? `myFlights.events.${type}` : "myFlights.events.information";
}
