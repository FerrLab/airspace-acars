import { describe, it, expect } from "vitest";
import {
  interpolateGreatCircle,
  generateFlightTrackCoordinates,
  nearestLongitude,
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

    // atan2 keeps longitudes in ±180, which made a transpacific route jump
    // from +180 to -180: Leaflet drew a line across the whole map and zoomed
    // out to the world.
    it.each([
      ["RJTT to KLAX", 35.552258, 139.779694, 33.942536, -118.408075],
      ["NZAA to PHNL", -37.008056, 174.791667, 21.318681, -157.922428],
      ["KLAX to RJTT", 33.942536, -118.408075, 35.552258, 139.779694],
    ])("keeps %s continuous across the antimeridian", (_name, lat1, lon1, lat2, lon2) => {
      const path = interpolateGreatCircle(lat1, lon1, lat2, lon2);
      for (let i = 1; i < path.length; i++) {
        expect(Math.abs(path[i][1] - path[i - 1][1]), `step ${i}`).toBeLessThan(20);
      }
      const lons = path.map((p) => p[1]);
      expect(Math.max(...lons) - Math.min(...lons)).toBeLessThan(180);
      // It ends at the arrival, on the neighbouring copy of the world.
      expect(nearestLongitude(lon2, path[path.length - 1][1])).toBeCloseTo(path[path.length - 1][1], 3);
      expect(path[path.length - 1][0]).toBeCloseTo(lat2, 3);
    });
  });

  describe("nearestLongitude", () => {
    it("moves a longitude by whole turns to within 180° of the reference", () => {
      expect(nearestLongitude(-118, 140)).toBe(242);
      expect(nearestLongitude(140, -118)).toBe(-220);
      expect(nearestLongitude(-46, -50)).toBe(-46);
    });
  });

  describe("generateFlightTrackCoordinates", () => {
    it("draws the great circle between departure and arrival", () => {
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
