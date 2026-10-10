import { describe, it, expect } from "vitest";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import {
  formatDurationHoursMinutes,
  formatScoreTier,
  computeLandingRateCategory,
  getFlightKPIs,
  getFlightFacts,
  getScoreCategories,
  getStabilizedGate,
  getAuditLogs,
  generateFlightTelemetry,
  getRunwayPerformance,
} from "./flight-detail";

const sampleFlight: FlightLog = {
  id: "880",
  callsign: "SXB9404",
  flight_number: "4J9404",
  status: "accepted",
  departure_airport: {
    icao: "SBGR",
    name: "Guarulhos",
    city: "São Paulo",
    latitude: -23.4356,
    longitude: -46.4731,
  },
  arrival_airport: {
    icao: "SBFI",
    name: "Cataratas",
    city: "Foz do Iguaçu",
    latitude: -25.5958,
    longitude: -54.4875,
  },
  aircraft: {
    registration: "PR-SBG",
    name: "Airbus A320ceo IAE",
    icao_code: "A320",
  },
  flight_time_minutes: 80,
  distance_nm: 456,
  landing_rate_fpm: -508,
  fuel_used_kg: 3427,
  score: 92,
  created_at: "2026-10-08T14:36:00Z",
  departure_time: "2026-10-08T14:36:00Z",
  arrival_time: "2026-10-08T16:10:00Z",
};

describe("flight-detail logic", () => {
  it("formats duration properly", () => {
    expect(formatDurationHoursMinutes(0)).toBe("—");
    expect(formatDurationHoursMinutes(45)).toBe("45m");
    expect(formatDurationHoursMinutes(80)).toBe("1h 20m");
    expect(formatDurationHoursMinutes(125)).toBe("2h 05m");
  });

  it("calculates score tier correctly", () => {
    expect(formatScoreTier(95).tier).toBe("Gold");
    expect(formatScoreTier(80).tier).toBe("Silver");
    expect(formatScoreTier(60).tier).toBe("Bronze");
  });

  it("classifies landing rates accurately", () => {
    expect(computeLandingRateCategory(-80).label).toBe("Butter");
    expect(computeLandingRateCategory(-180).label).toBe("Good");
    expect(computeLandingRateCategory(-320).label).toBe("Fair");
    expect(computeLandingRateCategory(-508).label).toBe("Hard");
    expect(computeLandingRateCategory(0).label).toBe("—");
  });

  it("extracts 6 key flight KPIs", () => {
    const kpis = getFlightKPIs(sampleFlight);
    expect(kpis).toHaveLength(6);
    expect(kpis[0].value).toBe("-508"); // landing rate
    expect(kpis[2].value).toBe("456"); // distance
    expect(kpis[3].value).toBe("3,427"); // fuel
    expect(kpis[4].value).toBe("1h 20m"); // duration
  });

  it("extracts flight facts for display", () => {
    const facts = getFlightFacts(sampleFlight);
    expect(facts.length).toBeGreaterThanOrEqual(6);
    expect(facts.find((f) => f.defaultLabel === "Número do Voo")?.value).toBe("4J9404");
    expect(facts.find((f) => f.defaultLabel === "Matrícula")?.value).toBe("PR-SBG");
  });

  it("calculates 5 score categories breakdown", () => {
    const cats = getScoreCategories(sampleFlight);
    expect(cats).toHaveLength(5);
    const otp = cats.find((c) => c.key === "otp");
    expect(otp).toBeDefined();
    expect(otp?.score).toBeGreaterThan(0);
  });

  it("provides stabilized gate checklist", () => {
    const gate = getStabilizedGate();
    expect(gate).toHaveLength(4);
    expect(gate.every((g) => g.passed)).toBe(true);
  });

  it("provides audit activity log items", () => {
    const logs = getAuditLogs(sampleFlight);
    expect(logs.length).toBeGreaterThan(0);
    expect(logs[0].by).toBe("FDM Engine");
  });

  it("generates deterministic synchronized telemetry data", () => {
    const telemetry = generateFlightTelemetry(sampleFlight);
    expect(telemetry.length).toBeGreaterThan(10);
    expect(telemetry[0].phase).toBe("Taxi");
    expect(telemetry[telemetry.length - 1].phase).toBe("Landing");

    // Deterministic: repeating on same flight produces identical values
    const secondPass = generateFlightTelemetry(sampleFlight);
    expect(secondPass[10].alt).toBe(telemetry[10].alt);
  });

  it("computes runway performance for landing and takeoff", () => {
    const landing = getRunwayPerformance(sampleFlight, "landing");
    expect(landing.runwayId).toBe("14");
    expect(landing.rateFpm).toBe(-508);

    const takeoff = getRunwayPerformance(sampleFlight, "takeoff");
    expect(takeoff.runwayId).toBe("10L");
    expect(takeoff.distanceM).toBeGreaterThan(1000);
  });
});
