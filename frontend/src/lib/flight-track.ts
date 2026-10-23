/**
 * Flight Track Generation Utility
 *
 * Computes great-circle (orthodromic) geodesic routes using spherical
 * interpolation (Slerp) between a flight's departure and arrival.
 */

export interface FlightTrackOptions {
  depLat: number;
  depLon: number;
  arrLat: number;
  arrLon: number;
  numPoints?: number;
}

/**
 * nearestLongitude returns lon moved by whole turns to within 180° of
 * reference. Leaflet draws a longitude beyond ±180 on the neighbouring copy of
 * the world, so a route kept continuous this way crosses the antimeridian as
 * a short line instead of jumping across the whole map.
 */
export function nearestLongitude(lon: number, reference: number): number {
  return lon - 360 * Math.round((lon - reference) / 360);
}

/**
 * Spherical Slerp (Great-Circle geodesic interpolation) between two coordinates.
 * Computes shortest-path trajectory points on Earth. Longitudes are continuous
 * from the departure's, so a transpacific route may end beyond ±180.
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
      [lat2, nearestLongitude(lon2, lon1)],
    ];
  }

  const sinOmega = Math.sin(omega);
  const points: [number, number][] = [];
  let previousLon = lon1;

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
    // atan2 keeps longitudes in ±180; carry on from the previous point instead.
    const lon = nearestLongitude(Math.atan2(y, x) * toDeg, previousLon);
    previousLon = lon;

    points.push([
      Math.round(phi * toDeg * 100000) / 100000,
      Math.round(lon * 100000) / 100000,
    ]);
  }

  return points;
}

/**
 * Generates the flight track coordinate sequence: the great circle between
 * departure and arrival, or nothing when either is not placed.
 */
export function generateFlightTrackCoordinates(
  options: FlightTrackOptions
): [number, number][] {
  const { depLat, depLon, arrLat, arrLon, numPoints = 32 } = options;

  const hasDep = !isNaN(depLat) && !isNaN(depLon) && (depLat !== 0 || depLon !== 0);
  const hasArr = !isNaN(arrLat) && !isNaN(arrLon) && (arrLat !== 0 || arrLon !== 0);

  if (!hasDep || !hasArr) {
    return [];
  }

  return interpolateGreatCircle(depLat, depLon, arrLat, arrLon, numPoints);
}
