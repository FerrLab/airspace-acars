import { describe, expect, it } from "vitest";
import type { FlightDetail, LandingReport, TakeoffReport } from "../../bindings/airspace-acars/internal/domain/models";
import {
  availableTabs,
  centrelineOffset,
  eventTypeKey,
  landingStrip,
  takeoffStrip,
  touchdownDistance,
  windComponents,
} from "./flight-detail-sections";

function detail(overrides: Partial<FlightDetail> = {}): FlightDetail {
  return {
    id: "7",
    landing_g_force: null,
    landing: null,
    takeoff: null,
    score_breakdown: { score: null, contributing: [], categories: [] },
    events: [],
    points: { total: 0, entries: [] },
    ...overrides,
  } as FlightDetail;
}

const runway = { designator: "20L", magnetic_heading: 200, length_m: 2000, width_m: 45 };

// A tab exists only for what the platform recorded, so the pilot is never
// shown an empty Landing tab or a score with nothing behind it.
describe("availableTabs", () => {
  it("shows only Overview when the detail could not be loaded", () => {
    expect(availableTabs(null)).toEqual(["overview"]);
  });

  it("hides Landing, Takeoff and Score when nothing was recorded", () => {
    expect(availableTabs(detail())).toEqual(["overview", "telemetry"]);
  });

  it("shows every tab when every section was recorded", () => {
    const full = detail({
      landing: { touchdown_distance_m: 400 } as LandingReport,
      takeoff: { liftoff_distance_m: 1800 } as TakeoffReport,
      score_breakdown: { score: 9, contributing: ["safety"], categories: [{ key: "safety" }] } as FlightDetail["score_breakdown"],
    });
    expect(availableTabs(full)).toEqual(["overview", "landing", "takeoff", "telemetry", "score"]);
  });
});

describe("centrelineOffset", () => {
  it("reads right, left and on the centreline", () => {
    expect(centrelineOffset(1.26)).toEqual({ key: "myFlights.runway.rightOfCentreline", values: { value: "1.3" } });
    expect(centrelineOffset(-2)).toEqual({ key: "myFlights.runway.leftOfCentreline", values: { value: "2.0" } });
    expect(centrelineOffset(0.01)).toEqual({ key: "myFlights.runway.onCentreline" });
  });

  it("says nothing about an offset that was not measured", () => {
    expect(centrelineOffset(null)).toBeNull();
    expect(centrelineOffset(undefined)).toBeNull();
  });
});

describe("touchdownDistance", () => {
  it("names a touchdown short of the threshold", () => {
    expect(touchdownDistance(-35)).toEqual({ key: "myFlights.landing.shortOfThreshold", values: { value: 35 } });
    expect(touchdownDistance(420)).toEqual({ key: "myFlights.units.metres", values: { value: 420 } });
    expect(touchdownDistance(null)).toBeNull();
  });
});

describe("windComponents", () => {
  it("tells a tailwind and a crosswind from the left apart from their opposites", () => {
    const parts = windComponents({ direction_degrees: 20, speed_knots: 10, headwind_knots: -4.2, crosswind_knots: -3 });
    expect(parts?.along).toEqual({ key: "myFlights.wind.tailwind", values: { value: "4.2" } });
    expect(parts?.across).toEqual({ key: "myFlights.wind.crosswindLeft", values: { value: "3.0" } });
  });

  it("reads a wind straight down the runway as no crosswind", () => {
    const parts = windComponents({ direction_degrees: 200, speed_knots: 10, headwind_knots: 10, crosswind_knots: 0 });
    expect(parts?.along.key).toBe("myFlights.wind.headwind");
    expect(parts?.across).toEqual({ key: "myFlights.wind.noCrosswind" });
  });

  it("has nothing to say without a wind sample", () => {
    expect(windComponents(null)).toBeNull();
  });
});

describe("runway strips", () => {
  it("draws a landing from touchdown to the end of the rollout", () => {
    const strip = landingStrip({ touchdown_distance_m: 400, runway_used_m: 1500, runway } as LandingReport);
    expect(strip).toEqual({ lengthM: 2000, fromPct: 20, toPct: 75, markPct: 20, runwayEndPct: 100 });
  });

  it("stretches the drawing past the runway end instead of clipping an overrun", () => {
    const strip = landingStrip({ touchdown_distance_m: 1000, runway_used_m: 2500, runway } as LandingReport);
    expect(strip?.toPct).toBe(100);
    expect(strip?.runwayEndPct).toBe(80);
  });

  it("draws nothing without a runway length or a touchdown point", () => {
    expect(landingStrip({ touchdown_distance_m: 400, runway: null } as LandingReport)).toBeNull();
    expect(landingStrip({ touchdown_distance_m: null, runway } as LandingReport)).toBeNull();
  });

  it("draws a takeoff from the start of the roll to liftoff", () => {
    const strip = takeoffStrip({ liftoff_distance_m: 1800, takeoff_roll_m: 1600, runway } as TakeoffReport);
    expect(strip).toEqual({ lengthM: 2000, fromPct: 10, toPct: 90, markPct: 90, runwayEndPct: 100 });
  });
});

describe("eventTypeKey", () => {
  it("falls back to information for an unknown type", () => {
    expect(eventTypeKey("violation")).toBe("myFlights.events.violation");
    expect(eventTypeKey("something")).toBe("myFlights.events.information");
  });
});
