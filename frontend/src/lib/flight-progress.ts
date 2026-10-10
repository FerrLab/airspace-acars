/**
 * Aviation Great-Circle Distance and Flight Progress Utilities
 */

const NM_PER_DEGREE = 60;
const EARTH_RADIUS_NM = 3440.065;

export interface Coordinates {
  lat: number;
  lon: number;
}

export interface AirportLike {
  icao?: string;
  latitude?: number | string | null;
  longitude?: number | string | null;
  lat?: number | string | null;
  lon?: number | string | null;
  lng?: number | string | null;
  [key: string]: unknown;
}

/**
 * Built-in coordinate dictionary for major and common VA airports.
 * Ensures instant calculation even if backend payload omits coordinates.
 */
export const AIRPORT_COORDINATES_MAP: Record<string, Coordinates> = {
  // Brazil & South America
  SBGR: { lat: -23.4356, lon: -46.4731 },
  SAWH: { lat: -54.8433, lon: -68.2956 },
  SBRJ: { lat: -22.9105, lon: -43.1631 },
  SBGL: { lat: -22.8089, lon: -43.2436 },
  SBKP: { lat: -23.0074, lon: -47.1345 },
  SBSP: { lat: -23.6261, lon: -46.6564 },
  SBBR: { lat: -15.8692, lon: -47.9172 },
  SBCF: { lat: -19.6244, lon: -43.9719 },
  SBPA: { lat: -29.9939, lon: -51.1711 },
  SBCT: { lat: -25.5285, lon: -49.1758 },
  SBFZ: { lat: -3.7763, lon: -38.5326 },
  SBRF: { lat: -8.1258, lon: -34.9231 },
  SBSV: { lat: -12.9086, lon: -38.3225 },
  SBEG: { lat: -3.0358, lon: -60.0497 },
  SBBE: { lat: -1.3847, lon: -48.4789 },
  SBGO: { lat: -16.6322, lon: -49.2211 },
  SBCG: { lat: -20.4686, lon: -54.6725 },
  SBFL: { lat: -27.6703, lon: -48.5525 },
  SBNF: { lat: -26.8797, lon: -48.6514 },
  SBVT: { lat: -20.2581, lon: -40.2864 },
  SBMQ: { lat: 0.0507, lon: -51.0722 },
  SBCA: { lat: -24.9539, lon: -53.5008 },
  SBPS: { lat: -16.4389, lon: -39.0808 },
  SBIL: { lat: -14.8164, lon: -39.0331 },
  SBMO: { lat: -9.5108, lon: -35.7917 },
  SBAR: { lat: -10.984, lon: -37.0703 },
  SBJP: { lat: -7.1483, lon: -34.9503 },
  SBNT: { lat: -5.7689, lon: -35.3728 },
  SBSL: { lat: -2.5869, lon: -44.2361 },
  SBTE: { lat: -5.0597, lon: -42.8236 },
  SBPV: { lat: -8.7094, lon: -63.9028 },
  SBRB: { lat: -9.8689, lon: -67.8983 },
  SBCY: { lat: -15.6528, lon: -56.1169 },
  SBPL: { lat: -9.3625, lon: -40.5647 },
  SBJU: { lat: -7.2189, lon: -39.2703 },
  SBCH: { lat: -27.1344, lon: -52.6564 },
  SBUG: { lat: -29.7825, lon: -57.0378 },
  SBTT: { lat: -4.2497, lon: -69.9419 },
  SBAT: { lat: -9.8664, lon: -56.105 },
  SBCZ: { lat: -7.5997, lon: -72.7694 },
  SBBW: { lat: -15.8619, lon: -52.3892 },
  SAEZ: { lat: -34.8222, lon: -58.5358 },
  SABE: { lat: -34.5592, lon: -58.4156 },
  SCEL: { lat: -33.393, lon: -70.7858 },
  SUMU: { lat: -34.8384, lon: -56.0308 },
  SGAS: { lat: -25.2397, lon: -57.5197 },
  SLVR: { lat: -17.6447, lon: -63.1353 },
  SPJC: { lat: -12.0219, lon: -77.1143 },
  SKBO: { lat: 4.7016, lon: -74.1469 },
  SVMI: { lat: 10.6031, lon: -66.9906 },
  SEQM: { lat: -0.1292, lon: -78.3575 },
  MPTO: { lat: 9.0714, lon: -79.3835 },
  MMMX: { lat: 19.4361, lon: -99.0719 },
  // North America & Europe
  KMIA: { lat: 25.7959, lon: -80.287 },
  KJFK: { lat: 40.6398, lon: -73.7789 },
  KLAX: { lat: 33.9425, lon: -118.4081 },
  KORD: { lat: 41.9742, lon: -87.9073 },
  KATL: { lat: 33.6407, lon: -84.4277 },
  EGLL: { lat: 51.47, lon: -0.4543 },
  LFPG: { lat: 49.0097, lon: 2.5479 },
  EDDF: { lat: 50.0379, lon: 8.5622 },
  EHAM: { lat: 52.3105, lon: 4.7683 },
  LEMD: { lat: 40.4839, lon: -3.568 },
  LPPT: { lat: 38.7756, lon: -9.1354 },
};

/**
 * Calculates Great-Circle distance in Nautical Miles using the Haversine formula.
 */
export function calculateDistanceNm(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number
): number {
  if (
    Number.isNaN(lat1) ||
    Number.isNaN(lon1) ||
    Number.isNaN(lat2) ||
    Number.isNaN(lon2)
  ) {
    return 0;
  }

  if (lat1 === lat2 && lon1 === lon2) {
    return 0;
  }

  const toRad = Math.PI / 180;
  const dLat = (lat2 - lat1) * toRad;
  const dLon = (lon2 - lon1) * toRad;

  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(lat1 * toRad) *
      Math.cos(lat2 * toRad) *
      Math.sin(dLon / 2) *
      Math.sin(dLon / 2);

  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return Math.round(EARTH_RADIUS_NM * c * 10) / 10;
}

/**
 * Formats time in minutes to standard flight tracker format: "H:MM" or "0:MM".
 * Matches industry standards (e.g. 57 -> "0:57", 119 -> "1:59").
 */
export function formatFlightDuration(totalMinutes: number): string {
  if (!totalMinutes || totalMinutes <= 0 || Number.isNaN(totalMinutes)) {
    return "0:00";
  }
  const hours = Math.floor(totalMinutes / 60);
  const minutes = Math.floor(totalMinutes % 60);
  return `${hours}:${minutes.toString().padStart(2, "0")}`;
}

/**
 * Resolves coordinates for an airport, checking explicit fields first,
 * then falling back to standard ICAO catalog.
 */
export function resolveAirportCoordinates(
  airport?: AirportLike | null,
  fallbackIcao?: string
): Coordinates | null {
  if (airport) {
    const rawLat = airport.latitude ?? airport.lat;
    const rawLon = airport.longitude ?? airport.lon ?? airport.lng;

    if (rawLat !== undefined && rawLat !== null && rawLon !== undefined && rawLon !== null) {
      const lat = typeof rawLat === "string" ? parseFloat(rawLat) : Number(rawLat);
      const lon = typeof rawLon === "string" ? parseFloat(rawLon) : Number(rawLon);

      if (!Number.isNaN(lat) && !Number.isNaN(lon) && (lat !== 0 || lon !== 0)) {
        return { lat, lon };
      }
    }
  }

  const icao = (airport?.icao || fallbackIcao || "").trim().toUpperCase();
  if (icao && AIRPORT_COORDINATES_MAP[icao]) {
    return AIRPORT_COORDINATES_MAP[icao];
  }

  return null;
}

export interface FlightProgressParams {
  flightState: "idle" | "active" | "finishing";
  departureAirport?: AirportLike | null;
  arrivalAirport?: AirportLike | null;
  currentLat?: number | null;
  currentLon?: number | null;
  onGround?: boolean;
  groundSpeed?: number;
  initialRecordedPosition?: Coordinates | null;
  flightStartTime?: number | Date | string | null;
  currentTime?: number | Date | null;
}

export interface FlightProgressResult {
  progressPercent: number;
  totalDistanceNm: number | null;
  remainingDistanceNm: number | null;
  flownDistanceNm: number | null;
  statusPhase: "idle" | "taxi" | "airborne" | "approach" | "finished";
  elapsedMinutes: number;
  remainingMinutes: number;
  durationMinutes: number;
}

/**
 * Computes progressive flight progress (0% - 100%) and navigation telemetry.
 */
export function computeFlightProgress({
  flightState,
  departureAirport,
  arrivalAirport,
  currentLat,
  currentLon,
  onGround = false,
  groundSpeed = 0,
  initialRecordedPosition = null,
  flightStartTime = null,
  currentTime = null,
}: FlightProgressParams): FlightProgressResult {
  const nowMs =
    typeof currentTime === "number"
      ? currentTime
      : currentTime instanceof Date
        ? currentTime.getTime()
        : Date.now();

  let resolvedElapsedMin: number | null = null;
  if (flightStartTime) {
    const startMs =
      typeof flightStartTime === "number"
        ? flightStartTime
        : flightStartTime instanceof Date
          ? flightStartTime.getTime()
          : typeof flightStartTime === "string" && !Number.isNaN(Number(flightStartTime))
            ? Number(flightStartTime)
            : new Date(flightStartTime).getTime();

    if (!Number.isNaN(startMs) && startMs > 0 && nowMs >= startMs) {
      resolvedElapsedMin = Math.floor((nowMs - startMs) / 60000);
    }
  }

  // If flight is idle (not started), plane stays parked at start (0%)
  if (flightState === "idle") {
    const depCoords = resolveAirportCoordinates(departureAirport);
    const arrCoords = resolveAirportCoordinates(arrivalAirport);
    const totalDist =
      depCoords && arrCoords
        ? calculateDistanceNm(depCoords.lat, depCoords.lon, arrCoords.lat, arrCoords.lon)
        : null;

    const totalMin = totalDist ? Math.round((totalDist / 440) * 60) : 0;

    return {
      progressPercent: 0,
      totalDistanceNm: totalDist,
      remainingDistanceNm: totalDist,
      flownDistanceNm: 0,
      statusPhase: "idle",
      elapsedMinutes: 0,
      remainingMinutes: totalMin,
      durationMinutes: totalMin,
    };
  }

  // If flight is finishing, plane has completed journey (100%)
  if (flightState === "finishing") {
    const depCoords = resolveAirportCoordinates(departureAirport) ?? initialRecordedPosition;
    const arrCoords = resolveAirportCoordinates(arrivalAirport);
    const totalDist =
      depCoords && arrCoords
        ? calculateDistanceNm(depCoords.lat, depCoords.lon, arrCoords.lat, arrCoords.lon)
        : null;

    const totalMin = totalDist ? Math.round((totalDist / 440) * 60) : 0;
    const elapsed = resolvedElapsedMin ?? totalMin;

    return {
      progressPercent: 100,
      totalDistanceNm: totalDist,
      remainingDistanceNm: 0,
      flownDistanceNm: totalDist,
      statusPhase: "finished",
      elapsedMinutes: elapsed,
      remainingMinutes: 0,
      durationMinutes: elapsed,
    };
  }

  // Flight is active: compute progress from coordinates
  const depCoords =
    resolveAirportCoordinates(departureAirport) ?? initialRecordedPosition;
  const arrCoords = resolveAirportCoordinates(arrivalAirport);

  const hasCurrentPos =
    typeof currentLat === "number" &&
    typeof currentLon === "number" &&
    !Number.isNaN(currentLat) &&
    !Number.isNaN(currentLon) &&
    (currentLat !== 0 || currentLon !== 0);

  if (depCoords && arrCoords) {
    const totalDist = calculateDistanceNm(
      depCoords.lat,
      depCoords.lon,
      arrCoords.lat,
      arrCoords.lon
    );

    // Realistic cruise speed: during taxi/climb, groundSpeed is slow (140-200kt).
    // Using 170kt for 2,100NM produces 12+ hours instead of 4:45.
    // Use groundSpeed when established at cruise (>= 280kt), otherwise 440kt.
    const effectiveCruiseSpeed = groundSpeed >= 280 ? groundSpeed : 440;
    const totalMin = Math.round((totalDist / 440) * 60);

    if (totalDist > 0 && hasCurrentPos) {
      const remainingDist = calculateDistanceNm(
        currentLat,
        currentLon,
        arrCoords.lat,
        arrCoords.lon
      );

      // On departure ground: 0%
      if (onGround && groundSpeed < 40 && remainingDist >= totalDist - 5) {
        const elapsed = resolvedElapsedMin ?? 0;
        return {
          progressPercent: 0,
          totalDistanceNm: totalDist,
          remainingDistanceNm: totalDist,
          flownDistanceNm: 0,
          statusPhase: "taxi",
          elapsedMinutes: elapsed,
          remainingMinutes: totalMin,
          durationMinutes: elapsed + totalMin,
        };
      }

      // Touchdown at destination
      if (onGround && remainingDist < 5) {
        const elapsed = resolvedElapsedMin ?? totalMin;
        return {
          progressPercent: 100,
          totalDistanceNm: totalDist,
          remainingDistanceNm: 0,
          flownDistanceNm: totalDist,
          statusPhase: "finished",
          elapsedMinutes: elapsed,
          remainingMinutes: 0,
          durationMinutes: elapsed,
        };
      }

      // General flight progress along great-circle
      const flownDist = Math.max(0, totalDist - remainingDist);
      const rawPercent = (flownDist / totalDist) * 100;
      const progressPercent = Math.min(100, Math.max(0, Math.round(rawPercent * 10) / 10));

      const isApproach = remainingDist <= 25 && !onGround;
      const approachSpeed = groundSpeed > 80 ? groundSpeed : 160;
      const activeSpeed = isApproach ? approachSpeed : effectiveCruiseSpeed;

      const elapsedMin = resolvedElapsedMin ?? Math.max(0, Math.round((flownDist / activeSpeed) * 60));
      const remainingMin = Math.max(0, Math.round((remainingDist / activeSpeed) * 60));

      return {
        progressPercent,
        totalDistanceNm: totalDist,
        remainingDistanceNm: remainingDist,
        flownDistanceNm: flownDist,
        statusPhase: isApproach ? "approach" : onGround ? "taxi" : "airborne",
        elapsedMinutes: elapsedMin,
        remainingMinutes: remainingMin,
        durationMinutes: elapsedMin + remainingMin,
      };
    }
  }

  // Fallback when destination coords are not known
  const fallbackElapsed = resolvedElapsedMin ?? 0;
  if (onGround) {
    return {
      progressPercent: 0,
      totalDistanceNm: null,
      remainingDistanceNm: null,
      flownDistanceNm: null,
      statusPhase: "taxi",
      elapsedMinutes: fallbackElapsed,
      remainingMinutes: 0,
      durationMinutes: fallbackElapsed,
    };
  }

  // If airborne without known destination coordinates, display minimum active progress
  return {
    progressPercent: 10,
    totalDistanceNm: null,
    remainingDistanceNm: null,
    flownDistanceNm: null,
    statusPhase: "airborne",
    elapsedMinutes: fallbackElapsed,
    remainingMinutes: 0,
    durationMinutes: fallbackElapsed,
  };
}
