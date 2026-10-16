import { describe, it, expect } from "vitest";
import {
  interpolateGreatCircle,
  generateFlightTrackCoordinates,
} from "./flight-track";

describe("flight-track", () => {
  describe("interpolateGreatCircle", () => {
    it("returns single point when start and end are identical", () => {
      const result = interpolateGreatCircle(-23.4356, -46.4731, -23.4356, -46.4731);
      expect(result).toHaveLength(1);
      expect(result[0]).toEqual([-23.4356, -46.4731]);
    });

    it("generates correct number of points along spherical great circle arc", () => {
      const numPoints = 16;
      const result = interpolateGreatCircle(
        -23.4356,
        -46.4731,
        -25.5958,
        -54.4875,
        numPoints
      );
      expect(result).toHaveLength(numPoints + 1);
      // First point should be departure (rounded to 5 decimal places)
      expect(result[0][0]).toBeCloseTo(-23.4356, 4);
      expect(result[0][1]).toBeCloseTo(-46.4731, 4);
      // Last point should be arrival
      expect(result[result.length - 1][0]).toBeCloseTo(-25.5958, 4);
      expect(result[result.length - 1][1]).toBeCloseTo(-54.4875, 4);
    });

    it("handles antipodal or extremely close points gracefully", () => {
      const result = interpolateGreatCircle(0, 0, 0.0000001, 0.0000001);
      expect(result.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe("generateFlightTrackCoordinates", () => {
    it("uses real trackCoordinates when provided with at least 2 points", () => {
      const realTrack: [number, number][] = [
        [-23.43, -46.47],
        [-24.1, -49.5],
        [-25.59, -54.48],
      ];
      const result = generateFlightTrackCoordinates({
        depLat: -23.43,
        depLon: -46.47,
        arrLat: -25.59,
        arrLon: -54.48,
        trackCoordinates: realTrack,
      });
      expect(result).toBe(realTrack);
    });

    it("falls back to great-circle when trackCoordinates is empty or undefined", () => {
      const result = generateFlightTrackCoordinates({
        depLat: -23.43,
        depLon: -46.47,
        arrLat: -25.59,
        arrLon: -54.48,
        numPoints: 8,
      });
      expect(result).toHaveLength(9);
      expect(result[0][0]).toBeCloseTo(-23.43, 2);
      expect(result[8][0]).toBeCloseTo(-25.59, 2);
    });

    it("returns empty array if coordinates are invalid or missing", () => {
      const result = generateFlightTrackCoordinates({
        depLat: 0,
        depLon: 0,
        arrLat: -25.59,
        arrLon: -54.48,
      });
      expect(result).toEqual([]);
    });
  });
});
