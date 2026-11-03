import { describe, expect, it } from "vitest";
import type { TrackPoint } from "../../bindings/airspace-acars/internal/domain/models";
import { buildTelemetryChart, sampleAt, seriesPoints } from "./flight-telemetry";

function point(sentAt: string, altitude: number, speed: number, phase = ""): TrackPoint {
  return {
    sent_at: sentAt,
    phase,
    lat: 0,
    lng: 0,
    altitude,
    altitude_agl: null,
    indicated_airspeed: speed,
    ground_speed: speed,
    vertical_speed: 0,
    heading: null,
  } as TrackPoint;
}

const track = [
  point("2026-09-26T14:00:00Z", 0, 0, "Taxi"),
  point("2026-09-26T14:10:00Z", 5000, 250, "Climb"),
  point("2026-09-26T14:20:00Z", 10000, 300, "Climb"),
  point("2026-09-26T14:40:00Z", 10000, 300, "Cruise"),
];

// The chart is drawn only from the recorded samples; nothing is generated.
describe("buildTelemetryChart", () => {
  it("spans the recorded samples and their maxima", () => {
    const chart = buildTelemetryChart(track)!;
    expect(chart.samples).toHaveLength(4);
    expect(chart.endMs - chart.startMs).toBe(40 * 60 * 1000);
    expect(chart.maxAltitude).toBe(10000);
    expect(chart.maxSpeed).toBe(300);
  });

  it("folds contiguous phases into segments sized by their share of the flight", () => {
    const phases = buildTelemetryChart(track)!.phases;
    // Cruise is only the final sample, so it spans no time and draws nothing.
    expect(phases.map((p) => p.phase)).toEqual(["Taxi", "Climb"]);
    expect(phases[0]).toMatchObject({ startPct: 0, widthPct: 25 });
    expect(phases[1]).toMatchObject({ startPct: 25, widthPct: 75 });
  });

  it("draws nothing from fewer than two placeable samples", () => {
    expect(buildTelemetryChart([])).toBeNull();
    expect(buildTelemetryChart([track[0]])).toBeNull();
    expect(buildTelemetryChart([track[0], point("not a time", 1, 1)])).toBeNull();
  });

  it("orders samples by time even if they arrive out of order", () => {
    const chart = buildTelemetryChart([track[2], track[0], track[1]])!;
    expect(chart.samples.map((s) => s.altitude)).toEqual([0, 5000, 10000]);
  });
});

describe("seriesPoints", () => {
  it("maps time to x and the value to y from the bottom", () => {
    const chart = buildTelemetryChart([track[0], track[2]])!;
    expect(seriesPoints(chart, (s) => s.altitude, chart.maxAltitude, 100, 50)).toBe("0.0,50.0 100.0,0.0");
  });
});

describe("sampleAt", () => {
  it("picks the recorded sample nearest the cursor", () => {
    const chart = buildTelemetryChart(track)!;
    expect(sampleAt(chart, 0.3).altitude).toBe(5000);
    expect(sampleAt(chart, 2).altitude).toBe(10000);
  });
});
