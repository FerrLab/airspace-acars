import { describe, it, expect } from "vitest";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import {
  formatDurationHoursMinutes,
  computeLandingRateCategory,
  formatScoreTier,
  getFlightKPIs,
  getFlightFacts,
} from "./flight-detail";

const mockFlight: FlightLog = {
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
  landing_rate_fpm: -142,
  fuel_used_kg: 3427,
  score: 92,
  created_at: "2026-10-08T14:36:00Z",
  departure_time: "2026-10-08T14:36:00Z",
  arrival_time: "2026-10-08T16:10:00Z",
  route: "BITIS UZ25 KASUK",
};

describe("flight-detail lib", () => {
  describe("formatDurationHoursMinutes", () => {
    it("returns dash for non-positive values", () => {
      expect(formatDurationHoursMinutes(0)).toBe("—");
      expect(formatDurationHoursMinutes(-10)).toBe("—");
    });

    it("formats minutes only when under 1 hour", () => {
      expect(formatDurationHoursMinutes(45)).toBe("45m");
    });

    it("formats hours and minutes when 1 hour or more", () => {
      expect(formatDurationHoursMinutes(80)).toBe("1h 20m");
      expect(formatDurationHoursMinutes(65)).toBe("1h 05m");
    });
  });

  describe("computeLandingRateCategory", () => {
    it("classifies smooth/butter landings", () => {
      const cat = computeLandingRateCategory(-100);
      expect(cat.label).toBe("Butter");
      expect(cat.colorClass).toContain("emerald");
    });

    it("classifies good landings", () => {
      const cat = computeLandingRateCategory(-220);
      expect(cat.label).toBe("Good");
      expect(cat.colorClass).toContain("sky");
    });

    it("classifies firm landings", () => {
      const cat = computeLandingRateCategory(-480);
      expect(cat.label).toBe("Firm");
      expect(cat.colorClass).toContain("rose");
    });
  });

  describe("formatScoreTier", () => {
    it("returns Gold for 90+", () => {
      expect(formatScoreTier(95).tier).toBe("Gold");
      expect(formatScoreTier(9.5).tier).toBe("Gold");
    });

    it("returns Silver for 75-89", () => {
      expect(formatScoreTier(80).tier).toBe("Silver");
    });

    it("returns Bronze for <75", () => {
      expect(formatScoreTier(60).tier).toBe("Bronze");
    });
  });

  describe("getFlightKPIs", () => {
    it("extracts real KPIs directly from FlightLog data", () => {
      const kpis = getFlightKPIs(mockFlight);
      expect(kpis).toHaveLength(5);

      const touchdown = kpis.find((k) => k.labelKey === "myFlights.touchdownRate");
      expect(touchdown?.value).toBe("-142");
      expect(touchdown?.unit).toBe("ft/min");

      const dist = kpis.find((k) => k.labelKey === "myFlights.kpiTotalDistance");
      expect(dist?.value).toBe("456");
      expect(dist?.unit).toBe("NM");

      const fuel = kpis.find((k) => k.labelKey === "myFlights.fuelBurned");
      expect(fuel?.value).toBe("3,427");
      expect(fuel?.unit).toBe("kg");

      const dur = kpis.find((k) => k.labelKey === "myFlights.flightDuration");
      expect(dur?.value).toBe("1h 20m");

      const score = kpis.find((k) => k.labelKey === "myFlights.operationalScore");
      expect(score?.value).toBe("9.2");
      expect(score?.sub).toContain("Gold");
    });
  });

  describe("getFlightFacts", () => {
    it("extracts flight facts including route if present", () => {
      const facts = getFlightFacts(mockFlight);
      const callsign = facts.find((f) => f.labelKey === "myFlights.factCallsign");
      expect(callsign?.value).toBe("SXB9404");

      const reg = facts.find((f) => f.labelKey === "myFlights.factRegistration");
      expect(reg?.value).toBe("PR-SBG");

      const route = facts.find((f) => f.labelKey === "myFlights.routeLabel");
      expect(route?.value).toBe("BITIS UZ25 KASUK");
    });
  });
});
