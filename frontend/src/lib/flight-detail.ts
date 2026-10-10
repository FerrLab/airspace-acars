import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";

export type FlightDetailTab = "overview" | "landing" | "takeoff" | "telemetry" | "score" | "log";

export interface FlightFactItem {
  labelKey: string;
  defaultLabel: string;
  value: string;
}

export interface FlightKPI {
  labelKey: string;
  defaultLabel: string;
  value: string;
  unit: string;
  sub: string;
  colorClass: string;
}

export interface ScoreCategory {
  key: string;
  labelKey: string;
  defaultLabel: string;
  score: number;
  weight: number;
  color: string;
  formula: string;
  inputs: [string, string][];
  subs?: [string, number, number][];
}

export interface TelemetrySample {
  t: number; // minutes from departure
  alt: number; // altitude ft
  gs: number; // ground speed kt
  ias: number; // indicated airspeed kt
  vs: number; // vertical speed fpm
  pitch: number; // pitch deg
  n1: number; // engine N1 %
  fuel: number; // fuel remaining kg
  flaps: number; // flap setting 0-4
  gear: number; // 0=up, 1=down
  phase: string;
}

export interface FlightAuditEvent {
  timeAgo: string;
  eventKey: string;
  defaultEvent: string;
  by: string;
}

export interface StabilizedGateItem {
  labelKey: string;
  defaultLabel: string;
  value: string;
  passed: boolean;
}

export interface RunwayPerformance {
  airportIcao: string;
  runwayId: string;
  recipId: string;
  lengthM: number;
  widthM: number;
  heading: number;
  rateFpm: number;
  gForce: number;
  distanceM: number;
  rolloutM: number;
  rolloutEndM: number;
  offsetM: number;
  offsetText: string;
  groundSpeedKt: number;
  airSpeedKt: number;
  runwayUsedM: number;
  runwayUsedPct: number;
  windHeading: number;
  windSpeedKt: number;
  headwindKt: number;
  crosswindKt: number;
  crosswindSide: "left" | "right";
}

// Pseudo-random deterministic generator based on flight ID or callsign
function createRng(seedStr: string) {
  let hash = 0;
  for (let i = 0; i < seedStr.length; i++) {
    hash = (hash << 5) - hash + seedStr.charCodeAt(i);
    hash |= 0;
  }
  let seed = Math.abs(hash) || 880;
  return () => {
    seed = (seed * 16807) % 2147483647;
    return seed / 2147483647 - 0.5;
  };
}

export function formatDurationHoursMinutes(minutes: number): string {
  if (!minutes || minutes <= 0) return "—";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m}m`;
  return `${h}h ${String(m).padStart(2, "0")}m`;
}

export function formatScoreTier(score: number): { tier: "Gold" | "Silver" | "Bronze"; color: string } {
  if (score >= 90) return { tier: "Gold", color: "#e0a526" };
  if (score >= 75) return { tier: "Silver", color: "#94a3b8" };
  return { tier: "Bronze", color: "#b45309" };
}

export function computeLandingRateCategory(rate: number): {
  label: string;
  sub: string;
  colorClass: string;
} {
  const abs = Math.abs(rate);
  if (abs === 0) return { label: "—", sub: "Sem dados", colorClass: "text-muted-foreground" };
  if (abs <= 120) return { label: "Butter", sub: "≤ 120 ft/min · Suave", colorClass: "text-emerald-400" };
  if (abs <= 250) return { label: "Good", sub: "≤ 250 ft/min · Bom", colorClass: "text-sky-400" };
  if (abs <= 400) return { label: "Fair", sub: "≤ 400 ft/min · Normal", colorClass: "text-amber-400" };
  return { label: "Hard", sub: "> 400 ft/min · Firme", colorClass: "text-rose-400" };
}

export function getFlightKPIs(flight: FlightLog): FlightKPI[] {
  const rateCat = computeLandingRateCategory(flight.landing_rate_fpm);
  const scoreVal = flight.score > 0 ? (flight.score > 10 ? (flight.score / 10).toFixed(2) : flight.score.toFixed(2)) : "—";
  const tier = formatScoreTier(flight.score > 10 ? flight.score : flight.score * 10);

  return [
    {
      labelKey: "myFlights.touchdownRate",
      defaultLabel: "Toque na Pista",
      value: flight.landing_rate_fpm ? `${Math.round(flight.landing_rate_fpm)}` : "—",
      unit: "ft/min",
      sub: rateCat.sub,
      colorClass: rateCat.colorClass,
    },
    {
      labelKey: "myFlights.kpiTouchdownG",
      defaultLabel: "G-Force Toque",
      value: "1.16",
      unit: "G",
      sub: "Smooth · até 1.2 G",
      colorClass: "text-emerald-400",
    },
    {
      labelKey: "myFlights.kpiTotalDistance",
      defaultLabel: "Distância Total",
      value: flight.distance_nm > 0 ? `${Math.round(flight.distance_nm)}` : "—",
      unit: "NM",
      sub: flight.distance_nm > 0 ? `~${Math.round(flight.distance_nm * 1.852)} km` : "Rota planejada",
      colorClass: "text-foreground",
    },
    {
      labelKey: "myFlights.fuelBurned",
      defaultLabel: "Combustível",
      value: flight.fuel_used_kg > 0 ? `${Math.round(flight.fuel_used_kg).toLocaleString("en-US")}` : "—",
      unit: "kg",
      sub: "Trip Fuel",
      colorClass: "text-amber-400",
    },
    {
      labelKey: "myFlights.flightDuration",
      defaultLabel: "Duração de Voo",
      value: formatDurationHoursMinutes(flight.flight_time_minutes),
      unit: "",
      sub: `${flight.flight_time_minutes || 0} min voados`,
      colorClass: "text-sky-400",
    },
    {
      labelKey: "myFlights.operationalScore",
      defaultLabel: "Score Operacional",
      value: scoreVal,
      unit: "/ 10",
      sub: `${tier.tier} Tier`,
      colorClass: "text-[#e0a526]",
    },
  ];
}

export function getFlightFacts(flight: FlightLog): FlightFactItem[] {
  const flightNum = flight.flight_number || flight.callsign;
  const aircraftName = flight.aircraft ? `${flight.aircraft.icao_code || ""} ${flight.aircraft.name ? `(${flight.aircraft.name})` : ""}`.trim() : "—";

  return [
    { labelKey: "myFlights.factFlightNumber", defaultLabel: "Número do Voo", value: flightNum },
    { labelKey: "myFlights.factCallsign", defaultLabel: "Callsign", value: flight.callsign },
    { labelKey: "myFlights.factRegistration", defaultLabel: "Matrícula", value: flight.aircraft?.registration || "—" },
    { labelKey: "myFlights.aircraftUsed", defaultLabel: "Aeronave", value: aircraftName || "—" },
    { labelKey: "myFlights.factDepartureAirport", defaultLabel: "Origem", value: `${flight.departure_airport?.icao} - ${flight.departure_airport?.city || flight.departure_airport?.name || ""}` },
    { labelKey: "myFlights.factArrivalAirport", defaultLabel: "Destino", value: `${flight.arrival_airport?.icao} - ${flight.arrival_airport?.city || flight.arrival_airport?.name || ""}` },
    { labelKey: "myFlights.factAirTime", defaultLabel: "Tempo de Voo", value: formatDurationHoursMinutes(flight.flight_time_minutes) },
    { labelKey: "myFlights.factStatus", defaultLabel: "Status do Voo", value: flight.status || "Accepted" },
  ];
}

export function getScoreCategories(flight: FlightLog): ScoreCategory[] {
  const baseScore = flight.score > 10 ? flight.score / 10 : flight.score || 9.2;
  const landingRate = Math.abs(flight.landing_rate_fpm || 180);
  const comfortSub = Math.min(10, Math.max(1, 10 - (landingRate > 200 ? (landingRate - 200) / 40 : 0)));

  return [
    {
      key: "otp",
      labelKey: "myFlights.scoreOTP",
      defaultLabel: "On-Time Performance",
      score: Math.min(10, +(baseScore * 1.05).toFixed(2)),
      weight: 20,
      color: "#0ea5e9",
      formula: "Within ±5 minutes of target = 10/10. Exponential decay applied beyond tolerance.",
      inputs: [
        ["Departure delta", "+6 min"],
        ["Arrival delta", "-4 min"],
        ["Departure sub-score", "9.84 / 10"],
        ["Arrival sub-score", "9.20 / 10"],
      ],
    },
    {
      key: "eco",
      labelKey: "myFlights.scoreEco",
      defaultLabel: "Eco Flight",
      score: Math.min(10, +(baseScore * 0.94).toFixed(2)),
      weight: 30,
      color: "#10b981",
      formula: "max(0, 10 − max(0, (actual/planned − 1) × 100)) × eco trust factor.",
      inputs: [
        ["Planned enroute fuel burn", `${Math.round(flight.fuel_used_kg * 0.98 || 3200)} kg`],
        ["Actual enroute fuel burn", `${Math.round(flight.fuel_used_kg || 3250)} kg`],
        ["Fuel ratio actual/planned", "1.015"],
        ["Trust factor", "1.00"],
      ],
    },
    {
      key: "manual",
      labelKey: "myFlights.scoreManual",
      defaultLabel: "Manual Flight",
      score: 8.5,
      weight: 10,
      color: "#a855f7",
      formula: "Hand-flown fraction of airborne time × 20, capped at 10.",
      inputs: [
        ["Hand-flown departure", "3.2 min"],
        ["Hand-flown approach", "4.8 min"],
        ["Total manual time", "8.0 min"],
      ],
    },
    {
      key: "comfort",
      labelKey: "myFlights.scoreComfort",
      defaultLabel: "Passenger Comfort",
      score: +comfortSub.toFixed(2),
      weight: 15,
      color: "#f59e0b",
      formula: "40% landing rate + 30% touchdown G + 15% max bank + 15% max G excursion.",
      inputs: [
        ["Landing rate", `${Math.round(flight.landing_rate_fpm || -150)} fpm`],
        ["Touchdown G", "1.16 G"],
        ["Max bank in cruise", "26.4° (limit 35°)"],
        ["Max G excursion", "0.22 G"],
      ],
      subs: [
        ["Landing rate", +comfortSub.toFixed(2), 40],
        ["Touchdown G", 10.0, 30],
        ["Max bank", 9.8, 15],
        ["G excursion", 9.0, 15],
      ],
    },
    {
      key: "safety",
      labelKey: "myFlights.scoreSafety",
      defaultLabel: "Safety Record",
      score: 10.0,
      weight: 25,
      color: "#22c55e",
      formula: "10 if no FDM warnings or violations were detected, otherwise penalty deductions.",
      inputs: [
        ["FDM warnings", "0"],
        ["FDM violations", "0"],
        ["Overspeed events", "0"],
        ["Flap exceedances", "0"],
      ],
    },
  ];
}

export function getStabilizedGate(): StabilizedGateItem[] {
  return [
    { labelKey: "myFlights.gateFlaps", defaultLabel: "Flaps at or above 70% (Landing config)", value: "FULL / 100%", passed: true },
    { labelKey: "myFlights.gateGear", defaultLabel: "Landing Gear Locked & Down", value: "DOWN & LOCKED", passed: true },
    { labelKey: "myFlights.gateSpeed", defaultLabel: "Speed within target reference (Vref + 5)", value: "+3 kt (within 30 kt)", passed: true },
    { labelKey: "myFlights.gateGlidepath", defaultLabel: "Glidepath within ±1 dot tolerance", value: "ON GLIDEPATH", passed: true },
  ];
}

export function getAuditLogs(flight: FlightLog): FlightAuditEvent[] {
  return [
    { timeAgo: "1m ago", eventKey: "myFlights.logProcessed", defaultEvent: "Flight telemetry analysis complete", by: "FDM Engine" },
    { timeAgo: "2m ago", eventKey: "myFlights.logApproved", defaultEvent: `Flight marked ${flight.status || "Approved"}`, by: "System" },
    { timeAgo: "3m ago", eventKey: "myFlights.logEventsDetected", defaultEvent: "FDM: 3 informational events detected, 0 violations", by: "Safety Service" },
    { timeAgo: "4m ago", eventKey: "myFlights.logSubmitted", defaultEvent: "Flight submitted by ACARS client", by: flight.callsign },
    { timeAgo: "5m ago", eventKey: "myFlights.logCreated", defaultEvent: "Flight dispatch & booking created", by: "Pilot" },
  ];
}

export function generateFlightTelemetry(flight: FlightLog): TelemetrySample[] {
  const duration = Math.max(20, flight.flight_time_minutes || 60);
  const totalSteps = 60;
  const rng = createRng(flight.id || flight.callsign);
  const samples: TelemetrySample[] = [];

  const cruiseAlt = flight.distance_nm > 300 ? 35000 : flight.distance_nm > 150 ? 28000 : 18000;
  const initialFuel = flight.fuel_used_kg > 0 ? flight.fuel_used_kg * 2.2 : 6000;
  let fuelRemaining = initialFuel;
  const burnRate = (flight.fuel_used_kg || 2500) / totalSteps;

  for (let i = 0; i <= totalSteps; i++) {
    const frac = i / totalSteps;
    const t = Math.round(frac * duration * 10) / 10;
    fuelRemaining = Math.max(500, fuelRemaining - burnRate);

    let alt = 0;
    let gs = 0;
    let ias = 0;
    let vs = 0;
    let pitch = 0;
    let n1 = 20;
    let flaps = 0;
    let gear = 0;
    let phase = "Taxi";

    if (frac < 0.05) {
      phase = "Taxi";
      alt = 750;
      gs = 15;
      ias = 0;
      vs = 0;
      pitch = 0;
      n1 = 22;
      flaps = 1;
      gear = 1;
    } else if (frac < 0.12) {
      phase = "Takeoff";
      const sub = (frac - 0.05) / 0.07;
      alt = 750 + sub * 2000;
      gs = 40 + sub * 140;
      ias = 30 + sub * 135;
      vs = 2200;
      pitch = 12;
      n1 = 88;
      flaps = 1;
      gear = sub < 0.5 ? 1 : 0;
    } else if (frac < 0.35) {
      phase = "Climb";
      const sub = (frac - 0.12) / 0.23;
      alt = 2750 + sub * (cruiseAlt - 2750);
      gs = 180 + sub * 260;
      ias = 250 + (sub < 0.5 ? sub * 40 : 20);
      vs = 1800 - sub * 800;
      pitch = 7 - sub * 4;
      n1 = 85;
      flaps = 0;
      gear = 0;
    } else if (frac < 0.7) {
      phase = "Cruise";
      alt = cruiseAlt + rng() * 60;
      gs = 440 + rng() * 12;
      ias = 270 + rng() * 5;
      vs = Math.round(rng() * 40);
      pitch = 2.5;
      n1 = 78;
      flaps = 0;
      gear = 0;
    } else if (frac < 0.9) {
      phase = "Descent";
      const sub = (frac - 0.7) / 0.2;
      alt = cruiseAlt - sub * (cruiseAlt - 3000);
      gs = 440 - sub * 200;
      ias = 270 - sub * 50;
      vs = -1800;
      pitch = -1.5;
      n1 = 38;
      flaps = sub > 0.8 ? 1 : 0;
      gear = 0;
    } else if (frac < 0.97) {
      phase = "Approach";
      const sub = (frac - 0.9) / 0.07;
      alt = 3000 - sub * 2250;
      gs = 240 - sub * 100;
      ias = 220 - sub * 80;
      vs = -750;
      pitch = 2.8;
      n1 = 56;
      flaps = sub > 0.5 ? 3 : 2;
      gear = 1;
    } else {
      phase = "Landing";
      const sub = (frac - 0.97) / 0.03;
      alt = 750;
      gs = Math.max(15, 140 - sub * 120);
      ias = Math.max(0, 135 - sub * 135);
      vs = sub === 0 ? flight.landing_rate_fpm || -160 : 0;
      pitch = 4.5 * (1 - sub);
      n1 = sub < 0.5 ? 70 : 22; // reverse thrust then idle
      flaps = 4;
      gear = 1;
    }

    samples.push({
      t,
      alt: Math.round(alt),
      gs: Math.round(gs),
      ias: Math.round(ias),
      vs: Math.round(vs),
      pitch: Math.round(pitch * 10) / 10,
      n1: Math.round(n1),
      fuel: Math.round(fuelRemaining),
      flaps,
      gear,
      phase,
    });
  }

  return samples;
}

const AIRPORT_RUNWAYS: Record<string, { id: string; recip: string; hdg: number; len: number; width: number }> = {
  SBFI: { id: "33", recip: "15", hdg: 328, len: 2800, width: 45 },
  SBGR: { id: "10L", recip: "28R", hdg: 95, len: 3700, width: 45 },
  SBSP: { id: "17R", recip: "35L", hdg: 172, len: 1940, width: 45 },
  SBRJ: { id: "02R", recip: "20L", hdg: 22, len: 1323, width: 42 },
  SBKP: { id: "15", recip: "33", hdg: 147, len: 3240, width: 45 },
  SBGL: { id: "10", recip: "28", hdg: 98, len: 4000, width: 45 },
  SBCF: { id: "16", recip: "34", hdg: 161, len: 3600, width: 45 },
  SBPA: { id: "11", recip: "29", hdg: 111, len: 3200, width: 45 },
  SBCT: { id: "15", recip: "33", hdg: 153, len: 2218, width: 45 },
  SBSV: { id: "10", recip: "28", hdg: 102, len: 3003, width: 45 },
  SBRF: { id: "18", recip: "36", hdg: 184, len: 3000, width: 45 },
  SBBE: { id: "06", recip: "24", hdg: 62, len: 2800, width: 45 },
  SBBH: { id: "13", recip: "31", hdg: 126, len: 2505, width: 45 },
  SBBR: { id: "11R", recip: "29L", hdg: 109, len: 3300, width: 45 },
  SBGO: { id: "14", recip: "32", hdg: 139, len: 2500, width: 45 },
  SBNT: { id: "12", recip: "30", hdg: 124, len: 3000, width: 60 },
  SBFL: { id: "14", recip: "32", hdg: 142, len: 2400, width: 45 },
};

export function getRunwayPerformance(flight: FlightLog, mode: "landing" | "takeoff"): RunwayPerformance {
  const isTakeoff = mode === "takeoff";
  const raw = flight as any;
  const rawLanding = raw.landing || {};
  const rawTakeoff = raw.takeoff || {};
  const rawArrRwy = raw.arrRwy || {};
  const rawDepRwy = raw.depRwy || {};
  const rawWind = raw.wind || {};

  const airport = isTakeoff ? flight.departure_airport : flight.arrival_airport;
  const airportIcao = (airport?.icao || (isTakeoff ? "SBGR" : "SBFI")).toUpperCase();
  const knownRwy = AIRPORT_RUNWAYS[airportIcao] || (isTakeoff
    ? { id: "10L", recip: "28R", hdg: 95, len: 3700, width: 45 }
    : { id: "33", recip: "15", hdg: 328, len: 2800, width: 45 });

  const rwyObj = isTakeoff ? rawDepRwy : rawArrRwy;
  const runwayId = raw.runway_id || rwyObj.id || (isTakeoff ? rawTakeoff.rwy : rawLanding.rwy) || knownRwy.id;
  const recipId = rwyObj.recip || knownRwy.recip;
  const lengthM = rwyObj.length || knownRwy.len;
  const widthM = rwyObj.width || knownRwy.width;
  const heading = rwyObj.hdg || knownRwy.hdg;

  const rate = flight.landing_rate_fpm || (isTakeoff ? 2200 : -160);
  const gForce = isTakeoff ? (rawTakeoff.g || 1.05) : (rawLanding.g || 1.16);
  const distanceM = isTakeoff ? (rawTakeoff.roll || rawTakeoff.liftoff || 2245) : (rawLanding.tdDist || 575);

  let rolloutM = 40;
  if (isTakeoff) {
    rolloutM = (rawTakeoff.liftoff || 2445) - (rawTakeoff.roll || 2245);
  } else if (rawLanding.roll != null) {
    rolloutM = rawLanding.roll;
  } else if (rawLanding.used != null && rawLanding.used >= distanceM) {
    rolloutM = Math.round(rawLanding.used - distanceM);
  } else if (rawLanding.rolloutEnd != null) {
    rolloutM = Math.round(rawLanding.rolloutEnd - distanceM);
  }
  if (rolloutM <= 0) rolloutM = 40;

  const rolloutEndM = distanceM + rolloutM;
  const offsetM = isTakeoff ? (rawTakeoff.offsetM ?? -7.0) : (rawLanding.offsetM ?? -9.8);
  const offsetText = isTakeoff
    ? (rawTakeoff.offset || `${Math.abs(offsetM).toFixed(1)} m left`)
    : (rawLanding.offset || `${Math.abs(offsetM).toFixed(1)} m left`);

  const groundSpeedKt = isTakeoff ? (rawTakeoff.gs || 169) : (rawLanding.gs || 137);
  const airSpeedKt = isTakeoff ? (rawTakeoff.ias || 158) : (rawLanding.ias || 136);

  const runwayUsedM = isTakeoff ? (rawTakeoff.liftoff || 2445) : (rawLanding.used || rolloutEndM);
  const runwayUsedPct = Math.min(100, isTakeoff
    ? (rawTakeoff.usedPct || (runwayUsedM / lengthM) * 100)
    : (rawLanding.usedPct || (runwayUsedM / lengthM) * 100));

  const windHeading = rawWind.dir || 150;
  const windSpeedKt = rawWind.kt || 8;
  const headwindKt = rawWind.head || 7.9;
  const crosswindKt = rawWind.cross || 1.5;
  const crosswindSide = (rawWind.crossSide || "right") as "left" | "right";

  return {
    airportIcao,
    runwayId,
    recipId,
    lengthM,
    widthM,
    heading,
    rateFpm: isTakeoff ? 2200 : rate,
    gForce,
    distanceM,
    rolloutM,
    rolloutEndM,
    offsetM,
    offsetText,
    groundSpeedKt,
    airSpeedKt,
    runwayUsedM,
    runwayUsedPct,
    windHeading,
    windSpeedKt,
    headwindKt,
    crosswindKt,
    crosswindSide,
  };
}
