import { describe, expect, it } from "vitest";
import {
  calculateDistanceNm,
  resolveAirportCoordinates,
  computeFlightProgress,
  formatFlightDuration,
} from "./flight-progress";

describe("flight-progress", () => {
  describe("calculateDistanceNm", () => {
    it("returns 0 for identical coordinates", () => {
      expect(calculateDistanceNm(-23.4356, -46.4731, -23.4356, -46.4731)).toBe(0);
    });

    it("calculates realistic distance between SBGR and SAWH (~2122 NM)", () => {
      const dist = calculateDistanceNm(-23.4356, -46.4731, -54.8433, -68.2956);
      expect(dist).toBeGreaterThan(2000);
      expect(dist).toBeLessThan(2250);
    });

    it("handles invalid or NaN inputs cleanly", () => {
      expect(calculateDistanceNm(NaN, 0, 10, 10)).toBe(0);
      expect(calculateDistanceNm(0, NaN, 10, 10)).toBe(0);
    });
  });

  describe("resolveAirportCoordinates", () => {
    it("resolves from explicit latitude/longitude fields", () => {
      const coords = resolveAirportCoordinates({
        icao: "XXXX",
        latitude: -15.5,
        longitude: -47.2,
      });
      expect(coords).toEqual({ lat: -15.5, lon: -47.2 });
    });

    it("resolves from string lat/lon fields", () => {
      const coords = resolveAirportCoordinates({
        icao: "XXXX",
        lat: "-22.91",
        lon: "-43.16",
      });
      expect(coords).toEqual({ lat: -22.91, lon: -43.16 });
    });

    it("falls back to ICAO catalog when explicit coords are missing", () => {
      const coords = resolveAirportCoordinates({ icao: "SBGR" });
      expect(coords).toEqual({ lat: -23.4356, lon: -46.4731 });

      const sawh = resolveAirportCoordinates({ icao: "SAWH" });
      expect(sawh).toEqual({ lat: -54.8433, lon: -68.2956 });
    });

    it("returns null when airport cannot be resolved", () => {
      expect(resolveAirportCoordinates({ icao: "ZZZZ" })).toBeNull();
      expect(resolveAirportCoordinates(null)).toBeNull();
    });
  });

  describe("computeFlightProgress", () => {
    const sbgr = { icao: "SBGR", latitude: -23.4356, longitude: -46.4731 };
    const sawh = { icao: "SAWH", latitude: -54.8433, longitude: -68.2956 };

    it("returns 0% when flightState is idle", () => {
      const res = computeFlightProgress({
        flightState: "idle",
        departureAirport: sbgr,
        arrivalAirport: sawh,
      });
      expect(res.progressPercent).toBe(0);
      expect(res.statusPhase).toBe("idle");
      expect(res.totalDistanceNm).toBeGreaterThan(2000);
      expect(res.remainingDistanceNm).toBe(res.totalDistanceNm);
    });

    it("returns 100% when flightState is finishing", () => {
      const res = computeFlightProgress({
        flightState: "finishing",
        departureAirport: sbgr,
        arrivalAirport: sawh,
      });
      expect(res.progressPercent).toBe(100);
      expect(res.statusPhase).toBe("finished");
      expect(res.remainingDistanceNm).toBe(0);
    });

    it("returns 0% when active but stationary on departure ground", () => {
      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -23.4356,
        currentLon: -46.4731,
        onGround: true,
        groundSpeed: 10,
      });
      expect(res.progressPercent).toBe(0);
      expect(res.statusPhase).toBe("taxi");
    });

    it("calculates accurate midpoint progress (~50%) when aircraft is halfway", () => {
      // Midpoint between SBGR and SAWH is approx -39.1, -57.4
      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -39.1,
        currentLon: -57.4,
        onGround: false,
        groundSpeed: 450,
      });
      expect(res.progressPercent).toBeGreaterThanOrEqual(45);
      expect(res.progressPercent).toBeLessThanOrEqual(55);
      expect(res.statusPhase).toBe("airborne");
    });

    it("returns 100% when landed on ground at destination", () => {
      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -54.843,
        currentLon: -68.295,
        onGround: true,
        groundSpeed: 15,
      });
      expect(res.progressPercent).toBe(100);
      expect(res.statusPhase).toBe("finished");
    });

    it("identifies approach phase within 25 NM of destination while airborne", () => {
      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -54.7,
        currentLon: -68.1,
        onGround: false,
        groundSpeed: 160,
      });
      expect(res.statusPhase).toBe("approach");
      expect(res.progressPercent).toBeGreaterThan(95);
      expect(res.elapsedMinutes).toBeGreaterThan(0);
      expect(res.remainingMinutes).toBeGreaterThanOrEqual(0);
    });

    it("tracks real elapsed time from flightStartTime regardless of position", () => {
      const startMs = 1700000000000;
      const fifteenMinLater = startMs + 15 * 60 * 1000;

      // Even on ground taxiing, elapsed time is accurately 15 min
      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -23.4356,
        currentLon: -46.4731,
        onGround: true,
        groundSpeed: 15,
        flightStartTime: startMs,
        currentTime: fifteenMinLater,
      });
      expect(res.elapsedMinutes).toBe(15);
      expect(res.durationMinutes).toBeGreaterThan(15);
    });

    it("calculates realistic remaining duration even during low-speed climb (does not explode to 12h)", () => {
      const startMs = 1700000000000;
      const fiveMinLater = startMs + 5 * 60 * 1000;

      const res = computeFlightProgress({
        flightState: "active",
        departureAirport: sbgr,
        arrivalAirport: sawh,
        currentLat: -24.0,
        currentLon: -47.0,
        onGround: false,
        groundSpeed: 174, // low climb speed
        flightStartTime: startMs,
        currentTime: fiveMinLater,
      });
      expect(res.elapsedMinutes).toBe(5);
      // For ~2100 NM, remaining should be around 4-5 hours (~280 min), not 12 hours!
      expect(res.remainingMinutes).toBeLessThan(350);
      expect(res.remainingMinutes).toBeGreaterThan(240);
    });
  });

  describe("formatFlightDuration", () => {
    it("formats 0 minutes to 0:00", () => {
      expect(formatFlightDuration(0)).toBe("0:00");
      expect(formatFlightDuration(-5)).toBe("0:00");
    });

    it("formats minutes under an hour to 0:MM (e.g. 57 -> 0:57)", () => {
      expect(formatFlightDuration(57)).toBe("0:57");
      expect(formatFlightDuration(5)).toBe("0:05");
    });

    it("formats hours and minutes accurately (e.g. 119 -> 1:59, 63 -> 1:03)", () => {
      expect(formatFlightDuration(119)).toBe("1:59");
      expect(formatFlightDuration(63)).toBe("1:03");
      expect(formatFlightDuration(289)).toBe("4:49");
    });
  });
});

