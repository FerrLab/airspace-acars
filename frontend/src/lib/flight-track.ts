/**
 * Flight Track Generation Utility
 *
 * Computes great-circle (orthodromic) geodesic routes using spherical interpolation (Slerp),
 * or utilizes real telemetry GPS track coordinates from the server when available.
 */

export interface FlightTrackOptions {
  depLat: number;
  depLon: number;
  arrLat: number;
  arrLon: number;
  trackCoordinates?: [number, number][];
  numPoints?: number;
}

/**
 * Spherical Slerp (Great-Circle geodesic interpolation) between two coordinates.
 * Computes shortest-path trajectory points on Earth.
 */
export function interpolateGreatCircle(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
  numPoints = 32
): [number, number][] {
  if (lat1 === lat2 && lon1 === lon2) {
    return [[lat1, lon1]];
  }

  const toRad = Math.PI / 180;
  const toDeg = 180 / Math.PI;

  const phi1 = lat1 * toRad;
  const lambda1 = lon1 * toRad;
  const phi2 = lat2 * toRad;
  const lambda2 = lon2 * toRad;

  // Angular distance in radians
  const cosOmega =
    Math.sin(phi1) * Math.sin(phi2) +
    Math.cos(phi1) * Math.cos(phi2) * Math.cos(lambda2 - lambda1);
  const omega = Math.acos(Math.max(-1, Math.min(1, cosOmega)));

  if (omega < 1e-6) {
    return [
      [lat1, lon1],
      [lat2, lon2],
    ];
  }

  const sinOmega = Math.sin(omega);
  const points: [number, number][] = [];

  for (let i = 0; i <= numPoints; i++) {
    const f = i / numPoints;
    const a = Math.sin((1 - f) * omega) / sinOmega;
    const b = Math.sin(f * omega) / sinOmega;

    const x =
      a * Math.cos(phi1) * Math.cos(lambda1) +
      b * Math.cos(phi2) * Math.cos(lambda2);
    const y =
      a * Math.cos(phi1) * Math.sin(lambda1) +
      b * Math.cos(phi2) * Math.sin(lambda2);
    const z = a * Math.sin(phi1) + b * Math.sin(phi2);

    const phi = Math.atan2(z, Math.hypot(x, y));
    const lambda = Math.atan2(y, x);

    points.push([
      Math.round(phi * toDeg * 100000) / 100000,
      Math.round(lambda * toDeg * 100000) / 100000,
    ]);
  }

  return points;
}

/**
 * Generates the flight track coordinate sequence.
 * Uses real recorded telemetry breadcrumbs if present, or spherical great-circle interpolation.
 */
export function generateFlightTrackCoordinates(
  options: FlightTrackOptions
): [number, number][] {
  const { depLat, depLon, arrLat, arrLon, trackCoordinates, numPoints = 32 } =
    options;

  if (Array.isArray(trackCoordinates) && trackCoordinates.length >= 2) {
    return trackCoordinates;
  }

  const hasDep = !isNaN(depLat) && !isNaN(depLon) && (depLat !== 0 || depLon !== 0);
  const hasArr = !isNaN(arrLat) && !isNaN(arrLon) && (arrLat !== 0 || arrLon !== 0);

  if (!hasDep || !hasArr) {
    return [];
  }

  return interpolateGreatCircle(depLat, depLon, arrLat, arrLon, numPoints);
}
