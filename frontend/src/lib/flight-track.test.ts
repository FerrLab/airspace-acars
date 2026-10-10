import { describe, it, expect } from "vitest";
import {
  interpolateGreatCircle,
  parseRouteWaypoints,
  generateFlightTrackCoordinates,
  KNOWN_FIXES,
  AIRWAY_CORRIDORS,
} from "./flight-track";

describe("flight-track engine", () => {
  describe("interpolateGreatCircle", () => {
    it("handles identical departure and arrival coordinates", () => {
      const pts = interpolateGreatCircle(-23.435, -46.473, -23.435, -46.473);
      expect(pts).toEqual([[-23.435, -46.473]]);
    });

    it("generates smooth geodesic coordinates between SBFI and SBGR", () => {
      const pts = interpolateGreatCircle(-25.5961, -54.4872, -23.4356, -46.4731, 10);
      expect(pts.length).toBe(11);
      // First point is origin, last point is destination
      expect(pts[0][0]).toBeCloseTo(-25.5961, 3);
      expect(pts[0][1]).toBeCloseTo(-54.4872, 3);
      expect(pts[10][0]).toBeCloseTo(-23.4356, 3);
      expect(pts[10][1]).toBeCloseTo(-46.4731, 3);

      // Intermediate points stay strictly within realistic flight boundaries
      // (no absurd displacement into northern Paraná)
      for (const [lat, lon] of pts) {
        expect(lat).toBeLessThanOrEqual(-23.4);
        expect(lat).toBeGreaterThanOrEqual(-25.7);
        expect(lon).toBeGreaterThanOrEqual(-54.6);
        expect(lon).toBeLessThanOrEqual(-46.4);
      }
    });
  });

  describe("parseRouteWaypoints", () => {
    it("returns empty array when routeText is undefined or empty", () => {
      expect(parseRouteWaypoints(undefined)).toEqual([]);
      expect(parseRouteWaypoints("")).toEqual([]);
    });

    it("extracts known fixes from a flight route string", () => {
      const fixes = parseRouteWaypoints("BITIS UZ25 EREBO KASUK BCO");
      expect(fixes.length).toBe(4);
      expect(fixes[0]).toEqual(KNOWN_FIXES.BITIS);
      expect(fixes[1]).toEqual(KNOWN_FIXES.EREBO);
      expect(fixes[2]).toEqual(KNOWN_FIXES.KASUK);
      expect(fixes[3]).toEqual(KNOWN_FIXES.BCO);
    });

    it("ignores unknown airway names and tokens gracefully", () => {
      const fixes = parseRouteWaypoints("DCT UNKNOWN_FIX W123 BCO");
      expect(fixes.length).toBe(1);
      expect(fixes[0]).toEqual(KNOWN_FIXES.BCO);
    });
  });

  describe("generateFlightTrackCoordinates", () => {
    it("uses provided trackCoordinates directly when available", () => {
      const customTelemetry: [number, number][] = [
        [-25.596, -54.487],
        [-25.100, -53.500],
        [-23.435, -46.473],
      ];
      const result = generateFlightTrackCoordinates({
        depLat: -25.596,
        depLon: -54.487,
        arrLat: -23.435,
        arrLon: -46.473,
        depIcao: "SBFI",
        arrIcao: "SBGR",
        trackCoordinates: customTelemetry,
      });
      expect(result).toBe(customTelemetry);
    });

    it("routes SBFI-SBGR through UZ25 airway corridor", () => {
      const result = generateFlightTrackCoordinates({
        depLat: -25.5961,
        depLon: -54.4872,
        arrLat: -23.4356,
        arrLon: -46.4731,
        depIcao: "SBFI",
        arrIcao: "SBGR",
      });

      expect(result.length).toBeGreaterThan(20);
      // Origin and destination
      expect(result[0][0]).toBeCloseTo(-25.5961, 2);
      expect(result[result.length - 1][0]).toBeCloseTo(-23.4356, 2);

      // Verify trajectory passes near the actual airway fixes
      // BITIS: [-25.20, -53.75]
      const nearBitis = result.some(
        ([lat, lon]) => Math.abs(lat - -25.20) < 0.25 && Math.abs(lon - -53.75) < 0.35
      );
      expect(nearBitis).toBe(true);

      // BCO (Sorocaba): [-23.47, -47.48]
      const nearBco = result.some(
        ([lat, lon]) => Math.abs(lat - -23.47) < 0.25 && Math.abs(lon - -47.48) < 0.35
      );
      expect(nearBco).toBe(true);
    });

    it("routes through parsed routeText fixes when provided", () => {
      const result = generateFlightTrackCoordinates({
        depLat: -25.5961,
        depLon: -54.4872,
        arrLat: -23.4356,
        arrLon: -46.4731,
        depIcao: "ZZZZ",
        arrIcao: "YYYY",
        routeText: "BITIS KASUK",
      });

      expect(result.length).toBeGreaterThan(10);
      const nearKasuk = result.some(
        ([lat, lon]) => Math.abs(lat - -24.10) < 0.25 && Math.abs(lon - -50.75) < 0.35
      );
      expect(nearKasuk).toBe(true);
    });

    it("falls back to Great-Circle geodesic when no corridor is registered", () => {
      const result = generateFlightTrackCoordinates({
        depLat: 40.6413,
        depLon: -73.7781, // KJFK
        arrLat: 51.4700,
        arrLon: -0.4543,  // EGLL
        depIcao: "KJFK",
        arrIcao: "EGLL",
      });

      expect(result.length).toBeGreaterThan(15);
      expect(result[0][0]).toBeCloseTo(40.6413, 3);
      expect(result[result.length - 1][0]).toBeCloseTo(51.4700, 3);
    });

    it("returns empty array when coordinates are missing or zero", () => {
      const result = generateFlightTrackCoordinates({
        depLat: 0,
        depLon: 0,
        arrLat: 0,
        arrLon: 0,
      });
      expect(result).toEqual([]);
    });
  });
});
