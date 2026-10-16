/**
 * Aviation Great-Circle Distance and Flight Progress Utilities
 */

const EARTH_RADIUS_NM = 3440.065;

export interface Coordinates {
  lat: number;
  lon: number;
}

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

export interface FlightProgressParams {
  flightState: "idle" | "active" | "finishing";
  departureCoords?: Coordinates | null;
  arrivalCoords?: Coordinates | null;
  currentLat?: number | null;
  currentLon?: number | null;
  onGround?: boolean;
  groundSpeed?: number;
  initialRecordedPosition?: Coordinates | null;
  flightStartTime?: number | Date | string | null;
  currentTime?: number | Date | null;
  cruiseSpeed?: number;
}

export interface FlightProgressResult {
  progressPercent: number | null;
  totalDistanceNm: number | null;
  remainingDistanceNm: number | null;
  flownDistanceNm: number | null;
  statusPhase: "idle" | "taxi" | "airborne" | "approach" | "finished";
  elapsedMinutes: number;
  remainingMinutes: number | null;
  durationMinutes: number | null;
}

/**
 * Computes progressive flight progress (0% - 100%) and navigation telemetry.
 */
export function computeFlightProgress({
  flightState,
  departureCoords,
  arrivalCoords,
  currentLat,
  currentLon,
  onGround = false,
  groundSpeed = 0,
  initialRecordedPosition = null,
  flightStartTime = null,
  currentTime = null,
  cruiseSpeed,
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

  const plannedCruiseSpeed = cruiseSpeed && cruiseSpeed > 0 ? cruiseSpeed : 440;

  // If flight is idle (not started), plane stays parked at start (0%)
  if (flightState === "idle") {
    const totalDist =
      departureCoords && arrivalCoords
        ? calculateDistanceNm(departureCoords.lat, departureCoords.lon, arrivalCoords.lat, arrivalCoords.lon)
        : null;

    const totalMin = totalDist ? Math.round((totalDist / plannedCruiseSpeed) * 60) : 0;

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
    const depCoords = departureCoords ?? initialRecordedPosition;
    const arrCoords = arrivalCoords;
    const totalDist =
      depCoords && arrCoords
        ? calculateDistanceNm(depCoords.lat, depCoords.lon, arrCoords.lat, arrCoords.lon)
        : null;

    const totalMin = totalDist ? Math.round((totalDist / plannedCruiseSpeed) * 60) : 0;
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
  const depCoords = departureCoords ?? initialRecordedPosition;
  const arrCoords = arrivalCoords;

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

    // Speed calculation:
    // Once airborne, use actual ground speed (with a reasonable floor during initial climb).
    // For non-jets (e.g. C172 at 110 kt or King Air at 250 kt), this accurately reflects real ground speed.
    const minClimbFloor = cruiseSpeed ? Math.min(cruiseSpeed, 70) : 80;
    const effectiveAirborneSpeed = Math.max(groundSpeed, minClimbFloor);
    const activeCruiseSpeed = !onGround ? effectiveAirborneSpeed : plannedCruiseSpeed;
    const totalMin = Math.round((totalDist / activeCruiseSpeed) * 60);

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
      const approachSpeed = groundSpeed > 60 ? groundSpeed : (cruiseSpeed ? Math.min(cruiseSpeed, 140) : 160);
      const activeSpeed = isApproach ? approachSpeed : activeCruiseSpeed;

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

  // Fallback when destination or departure coordinates are not known.
  // Return an indeterminate state (no fake progress percentage, elapsed time only).
  const fallbackElapsed = resolvedElapsedMin ?? 0;
  if (onGround) {
    return {
      progressPercent: 0,
      totalDistanceNm: null,
      remainingDistanceNm: null,
      flownDistanceNm: null,
      statusPhase: "taxi",
      elapsedMinutes: fallbackElapsed,
      remainingMinutes: null,
      durationMinutes: fallbackElapsed,
    };
  }

  // If airborne without known destination coordinates, display indeterminate state
  return {
    progressPercent: null,
    totalDistanceNm: null,
    remainingDistanceNm: null,
    flownDistanceNm: null,
    statusPhase: "airborne",
    elapsedMinutes: fallbackElapsed,
    remainingMinutes: null,
    durationMinutes: fallbackElapsed,
  };
}
