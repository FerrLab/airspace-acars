import { describe, expect, it } from "vitest";
import {
  calculateDistanceNm,
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

  describe("computeFlightProgress", () => {
    const sbgr = { lat: -23.4356, lon: -46.4731 };
    const sawh = { lat: -54.8433, lon: -68.2956 };

    it("returns 0% when flightState is idle", () => {
      const res = computeFlightProgress({
        flightState: "idle",
        departureCoords: sbgr,
        arrivalCoords: sawh,
      });
      expect(res.progressPercent).toBe(0);
      expect(res.statusPhase).toBe("idle");
      expect(res.totalDistanceNm).toBeGreaterThan(2000);
      expect(res.remainingDistanceNm).toBe(res.totalDistanceNm);
    });

    it("returns 100% when flightState is finishing", () => {
      const res = computeFlightProgress({
        flightState: "finishing",
        departureCoords: sbgr,
        arrivalCoords: sawh,
      });
      expect(res.progressPercent).toBe(100);
      expect(res.statusPhase).toBe("finished");
      expect(res.remainingDistanceNm).toBe(0);
    });

    it("returns 0% when active but stationary on departure ground", () => {
      const res = computeFlightProgress({
        flightState: "active",
        departureCoords: sbgr,
        arrivalCoords: sawh,
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
        departureCoords: sbgr,
        arrivalCoords: sawh,
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
        departureCoords: sbgr,
        arrivalCoords: sawh,
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
        departureCoords: sbgr,
        arrivalCoords: sawh,
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

      const res = computeFlightProgress({
        flightState: "active",
        departureCoords: sbgr,
        arrivalCoords: sawh,
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

    it("calculates accurate ETA for slow general aviation aircraft (e.g. C172 at 110 kt)", () => {
      // 110 NM leg: departure at (0, 0), arrival at (0, 1.833) approx 110 NM away
      const dep = { lat: 0, lon: 0 };
      const arr = { lat: 0, lon: 1.833 };
      const halfway = { lat: 0, lon: 0.916 };

      const res = computeFlightProgress({
        flightState: "active",
        departureCoords: dep,
        arrivalCoords: arr,
        currentLat: halfway.lat,
        currentLon: halfway.lon,
        onGround: false,
        groundSpeed: 110,
        cruiseSpeed: 110,
      });

      // Halfway through 110 NM at 110 kt: remaining distance is ~55 NM -> remaining time should be ~30 min
      expect(res.progressPercent).toBeGreaterThanOrEqual(48);
      expect(res.progressPercent).toBeLessThanOrEqual(52);
      expect(res.remainingMinutes).toBeGreaterThanOrEqual(28);
      expect(res.remainingMinutes).toBeLessThanOrEqual(32);
    });

    it("calculates accurate ETA for turboprop aircraft (e.g. King Air / ATR at 250 kt)", () => {
      // 250 NM leg: departure at (0, 0), arrival at (0, 4.166) approx 250 NM away
      const dep = { lat: 0, lon: 0 };
      const arr = { lat: 0, lon: 4.166 };
      const halfway = { lat: 0, lon: 2.083 };

      const res = computeFlightProgress({
        flightState: "active",
        departureCoords: dep,
        arrivalCoords: arr,
        currentLat: halfway.lat,
        currentLon: halfway.lon,
        onGround: false,
        groundSpeed: 250,
      });

      // Halfway through 250 NM at 250 kt: remaining distance is ~125 NM -> remaining time should be ~30 min
      expect(res.progressPercent).toBeGreaterThanOrEqual(48);
      expect(res.progressPercent).toBeLessThanOrEqual(52);
      expect(res.remainingMinutes).toBeGreaterThanOrEqual(28);
      expect(res.remainingMinutes).toBeLessThanOrEqual(32);
    });

    it("returns indeterminate state without fake progress when destination coordinates are unknown", () => {
      const res = computeFlightProgress({
        flightState: "active",
        departureCoords: sbgr,
        arrivalCoords: null,
        currentLat: -30.0,
        currentLon: -50.0,
        onGround: false,
        groundSpeed: 400,
        flightStartTime: 1700000000000,
        currentTime: 1700000000000 + 12 * 60 * 1000,
      });

      expect(res.progressPercent).toBeNull();
      expect(res.remainingDistanceNm).toBeNull();
      expect(res.remainingMinutes).toBeNull();
      expect(res.elapsedMinutes).toBe(12);
      expect(res.statusPhase).toBe("airborne");
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
