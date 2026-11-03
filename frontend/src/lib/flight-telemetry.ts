/**
 * Chart geometry for the Telemetry tab, computed from the flight's recorded
 * track. The chart draws altitude and indicated airspeed against time, with a
 * ribbon of the flight phases the ACARS reported.
 */
import type { TrackPoint } from "../../bindings/airspace-acars/internal/domain/models";

/** One sample on the chart, in the units of the track. */
export interface TelemetrySample {
  timeMs: number;
  altitude: number;
  speed: number;
  phase: string;
}

/** A contiguous run of one phase, as a share of the chart's width. */
export interface PhaseSegment {
  phase: string;
  startPct: number;
  widthPct: number;
}

export interface TelemetryChart {
  samples: TelemetrySample[];
  startMs: number;
  endMs: number;
  maxAltitude: number;
  maxSpeed: number;
  phases: PhaseSegment[];
}

/**
 * Samples whose timestamp parses, in time order. The server already sends
 * them in order and drops sim glitches; this only guards against a sample
 * the chart cannot place.
 */
export function telemetrySamples(points: TrackPoint[]): TelemetrySample[] {
  return points
    .map((p) => ({
      timeMs: Date.parse(p.sent_at),
      altitude: p.altitude,
      speed: p.indicated_airspeed,
      phase: p.phase ?? "",
    }))
    .filter((s) => Number.isFinite(s.timeMs))
    .sort((a, b) => a.timeMs - b.timeMs);
}

/**
 * The chart for a track, or null when it has fewer than two placeable
 * samples (a single point draws no line).
 */
export function buildTelemetryChart(points: TrackPoint[]): TelemetryChart | null {
  const samples = telemetrySamples(points);
  if (samples.length < 2) return null;
  const startMs = samples[0].timeMs;
  const endMs = samples[samples.length - 1].timeMs;
  if (endMs <= startMs) return null;

  return {
    samples,
    startMs,
    endMs,
    maxAltitude: Math.max(1, ...samples.map((s) => s.altitude)),
    maxSpeed: Math.max(1, ...samples.map((s) => s.speed)),
    phases: phaseSegments(samples, startMs, endMs),
  };
}

function phaseSegments(samples: TelemetrySample[], startMs: number, endMs: number): PhaseSegment[] {
  const span = endMs - startMs;
  const segments: PhaseSegment[] = [];
  let runStart = samples[0];
  for (let i = 1; i <= samples.length; i++) {
    const sample = samples[i];
    if (sample && sample.phase === runStart.phase) continue;
    const runEnd = sample ? sample.timeMs : endMs;
    if (runStart.phase && runEnd > runStart.timeMs) {
      segments.push({
        phase: runStart.phase,
        startPct: ((runStart.timeMs - startMs) / span) * 100,
        widthPct: ((runEnd - runStart.timeMs) / span) * 100,
      });
    }
    if (sample) runStart = sample;
  }
  return segments;
}

/**
 * An SVG polyline `points` attribute for one series, in a viewBox of
 * width × height with y growing downwards.
 */
export function seriesPoints(
  chart: TelemetryChart,
  value: (s: TelemetrySample) => number,
  max: number,
  width: number,
  height: number,
): string {
  const span = chart.endMs - chart.startMs;
  return chart.samples
    .map((s) => {
      const x = ((s.timeMs - chart.startMs) / span) * width;
      const y = height - (Math.max(0, value(s)) / max) * height;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}

/** The sample nearest a horizontal position given as a 0–1 fraction. */
export function sampleAt(chart: TelemetryChart, fraction: number): TelemetrySample {
  const target = chart.startMs + Math.min(1, Math.max(0, fraction)) * (chart.endMs - chart.startMs);
  let best = chart.samples[0];
  for (const s of chart.samples) {
    if (Math.abs(s.timeMs - target) < Math.abs(best.timeMs - target)) best = s;
  }
  return best;
}
