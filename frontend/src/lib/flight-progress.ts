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

  // Before the aircraft is flying there is no speed to time the route by. The
  // booking carries no cruise speed, and assuming a jet's would show a C172 a
  // quarter of its real flight time, so without one no estimate is shown.
  const plannedCruiseSpeed = cruiseSpeed && cruiseSpeed > 0 ? cruiseSpeed : null;
  const plannedMinutes = (distNm: number | null) =>
    distNm && plannedCruiseSpeed ? Math.round((distNm / plannedCruiseSpeed) * 60) : null;

  // If flight is idle (not started), plane stays parked at start (0%)
  if (flightState === "idle") {
    const totalDist =
      departureCoords && arrivalCoords
        ? calculateDistanceNm(departureCoords.lat, departureCoords.lon, arrivalCoords.lat, arrivalCoords.lon)
        : null;
    const totalMin = plannedMinutes(totalDist);

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
    const totalDist =
      departureCoords && arrivalCoords
        ? calculateDistanceNm(departureCoords.lat, departureCoords.lon, arrivalCoords.lat, arrivalCoords.lon)
        : null;

    return {
      progressPercent: 100,
      totalDistanceNm: totalDist,
      remainingDistanceNm: 0,
      flownDistanceNm: totalDist,
      statusPhase: "finished",
      elapsedMinutes: resolvedElapsedMin ?? 0,
      remainingMinutes: 0,
      durationMinutes: resolvedElapsedMin,
    };
  }

  // Flight is active: compute progress from coordinates
  const depCoords = departureCoords;
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

    // Once airborne, the time left is timed by the actual ground speed, with a
    // floor so a slow initial climb does not blow the estimate up; that holds
    // for a C172 at 110 kt as much as for a jet. On the ground only a known
    // cruise speed is used.
    const minClimbFloor = cruiseSpeed ? Math.min(cruiseSpeed, 70) : 80;
    const airborneSpeed = Math.max(groundSpeed, minClimbFloor);
    // The whole flight's time is known only when both parts of it are.
    const totalTime = (remainingMin: number | null) =>
      resolvedElapsedMin !== null && remainingMin !== null ? resolvedElapsedMin + remainingMin : null;

    if (totalDist > 0 && hasCurrentPos) {
      const remainingDist = calculateDistanceNm(
        currentLat,
        currentLon,
        arrCoords.lat,
        arrCoords.lon
      );

      // On departure ground: 0%
      if (onGround && groundSpeed < 40 && remainingDist >= totalDist - 5) {
        const remainingMin = plannedMinutes(totalDist);
        return {
          progressPercent: 0,
          totalDistanceNm: totalDist,
          remainingDistanceNm: totalDist,
          flownDistanceNm: 0,
          statusPhase: "taxi",
          elapsedMinutes: resolvedElapsedMin ?? 0,
          remainingMinutes: remainingMin,
          durationMinutes: totalTime(remainingMin),
        };
      }

      // Touchdown at destination
      if (onGround && remainingDist < 5) {
        return {
          progressPercent: 100,
          totalDistanceNm: totalDist,
          remainingDistanceNm: 0,
          flownDistanceNm: totalDist,
          statusPhase: "finished",
          elapsedMinutes: resolvedElapsedMin ?? 0,
          remainingMinutes: 0,
          durationMinutes: resolvedElapsedMin,
        };
      }

      // General flight progress along great-circle
      const flownDist = Math.max(0, totalDist - remainingDist);
      const rawPercent = (flownDist / totalDist) * 100;
      const progressPercent = Math.min(100, Math.max(0, Math.round(rawPercent * 10) / 10));

      const isApproach = remainingDist <= 25 && !onGround;
      const approachSpeed = groundSpeed > 60 ? groundSpeed : (cruiseSpeed ? Math.min(cruiseSpeed, 140) : 160);
      const activeSpeed = onGround ? plannedCruiseSpeed : isApproach ? approachSpeed : airborneSpeed;
      const remainingMin = activeSpeed ? Math.max(0, Math.round((remainingDist / activeSpeed) * 60)) : null;

      return {
        progressPercent,
        totalDistanceNm: totalDist,
        remainingDistanceNm: remainingDist,
        flownDistanceNm: flownDist,
        statusPhase: isApproach ? "approach" : onGround ? "taxi" : "airborne",
        elapsedMinutes: resolvedElapsedMin ?? 0,
        remainingMinutes: remainingMin,
        durationMinutes: totalTime(remainingMin),
      };
    }
  }

  // Fallback when destination or departure coordinates are not known.
  // Return an indeterminate state (no fake progress percentage, elapsed time only).
  return {
    progressPercent: null,
    totalDistanceNm: null,
    remainingDistanceNm: null,
    flownDistanceNm: null,
    statusPhase: onGround ? "taxi" : "airborne",
    elapsedMinutes: resolvedElapsedMin ?? 0,
    remainingMinutes: null,
    durationMinutes: null,
  };
}

/**
 * progressPhaseKey names the translation key for the phase shown above the
 * progress bar. A flight that has landed and is taxiing in is still active,
 * but it has arrived.
 */
export function progressPhaseKey(
  flightState: FlightProgressParams["flightState"],
  statusPhase: FlightProgressResult["statusPhase"],
): string {
  if (flightState === "finishing" || statusPhase === "finished") return "acars.dashboard.completedPhase";
  if (flightState === "idle" || statusPhase === "taxi") return "acars.dashboard.preflightPhase";
  if (statusPhase === "approach") return "acars.dashboard.approachPhase";
  return "acars.dashboard.enRoute";
}
