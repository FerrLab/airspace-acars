import { describe, it, expect } from "vitest";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import {
  formatDurationHoursMinutes,
  formatScore,
  landingRateBand,
  getFlightKPIs,
  getFlightFacts,
} from "./flight-detail";

const mockFlight: FlightLog = {
  id: "880",
  callsign: "SXB9404",
  flight_number: "4J9404",
  status: "acceptedSystem",
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

  // The same touchdown must get the same word as on the platform's flight
  // page, whose LandingRateBand is <=100 butter, <=200 good, <=400 firm.
  describe("landingRateBand", () => {
    it.each([
      [-100, "myFlights.landingBand.butter"],
      [-110, "myFlights.landingBand.good"],
      [-200, "myFlights.landingBand.good"],
      [-400, "myFlights.landingBand.firm"],
      [-450, "myFlights.landingBand.hard"],
    ])("a %d fpm touchdown is %s", (rate, key) => {
      expect(landingRateBand(rate).labelKey).toBe(key);
    });

    it("names no band when no touchdown was recorded", () => {
      expect(landingRateBand(0).labelKey).toBeNull();
    });
  });

  // The server's score is the composite × 10, from 0 to 100. A guess that
  // read low scores as already out of 10 showed 8/100 as 8.0.
  describe("formatScore", () => {
    it.each([
      [92, "9.2"],
      [8, "0.8"],
      [100, "10.0"],
      [0, "—"],
    ])("a score of %d reads %s / 10", (score, shown) => {
      expect(formatScore(score)).toBe(shown);
    });
  });

  describe("getFlightKPIs", () => {
    it("extracts real KPIs directly from FlightLog data", () => {
      const kpis = getFlightKPIs(mockFlight, "en");
      expect(kpis).toHaveLength(5);

      const touchdown = kpis.find((k) => k.labelKey === "myFlights.touchdownRate");
      expect(touchdown?.value).toBe("-142");
      expect(touchdown?.unit).toBe("ft/min");
      expect(touchdown?.subKey).toBe("myFlights.landingBand.good");

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
      expect(score?.sub).toBeUndefined();
    });

    it("groups numbers the way the pilot's language does", () => {
      const fuel = getFlightKPIs(mockFlight, "pt").find((k) => k.labelKey === "myFlights.fuelBurned");
      expect(fuel?.value).toBe("3.427");
    });
  });

  describe("getFlightFacts", () => {
    it("extracts flight facts with the times in Zulu", () => {
      const facts = getFlightFacts(mockFlight);
      expect(facts.find((f) => f.labelKey === "myFlights.factCallsign")?.value).toBe("SXB9404");
      expect(facts.find((f) => f.labelKey === "myFlights.factRegistration")?.value).toBe("PR-SBG");
      expect(facts.find((f) => f.labelKey === "myFlights.factDepartureTime")?.value).toBe("08/10 14:36z");
      expect(facts.find((f) => f.labelKey === "myFlights.factArrivalTime")?.value).toBe("08/10 16:10z");
    });
  });
});
