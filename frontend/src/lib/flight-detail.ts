import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";

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
  sub?: string;
  colorClass: string;
}

export function formatDurationHoursMinutes(minutes: number): string {
  if (!minutes || minutes <= 0) return "—";
  const h = Math.floor(minutes / 60);
  const m = Math.round(minutes % 60);
  if (h === 0) return `${m}m`;
  return `${h}h ${String(m).padStart(2, "0")}m`;
}

export function formatScoreTier(score: number): {
  tier: "Gold" | "Silver" | "Bronze" | "";
  color: string;
} {
  if (score <= 0) return { tier: "", color: "" };
  const normalized = score > 10 ? score : score * 10;
  if (normalized >= 90) return { tier: "Gold", color: "#e0a526" };
  if (normalized >= 75) return { tier: "Silver", color: "#94a3b8" };
  return { tier: "Bronze", color: "#b45309" };
}

export function computeLandingRateCategory(rate: number): {
  label: string;
  colorClass: string;
} {
  const abs = Math.abs(rate);
  if (abs === 0) return { label: "—", colorClass: "text-muted-foreground" };
  if (abs <= 120) return { label: "Butter", colorClass: "text-emerald-400" };
  if (abs <= 250) return { label: "Good", colorClass: "text-sky-400" };
  if (abs <= 400) return { label: "Normal", colorClass: "text-amber-400" };
  return { label: "Firm", colorClass: "text-rose-400" };
}

export function getFlightKPIs(flight: FlightLog): FlightKPI[] {
  const rateCat = computeLandingRateCategory(flight.landing_rate_fpm);
  const scoreVal =
    flight.score > 0
      ? (flight.score > 10 ? flight.score / 10 : flight.score).toFixed(1)
      : "—";
  const tier = formatScoreTier(flight.score);

  return [
    {
      labelKey: "myFlights.touchdownRate",
      defaultLabel: "Touchdown Rate",
      value: flight.landing_rate_fpm
        ? `${Math.round(flight.landing_rate_fpm)}`
        : "—",
      unit: "ft/min",
      sub: rateCat.label !== "—" ? rateCat.label : undefined,
      colorClass: rateCat.colorClass,
    },
    {
      labelKey: "myFlights.kpiTotalDistance",
      defaultLabel: "Total Distance",
      value: flight.distance_nm > 0 ? `${Math.round(flight.distance_nm)}` : "—",
      unit: "NM",
      sub:
        flight.distance_nm > 0
          ? `~${Math.round(flight.distance_nm * 1.852)} km`
          : undefined,
      colorClass: "text-foreground",
    },
    {
      labelKey: "myFlights.fuelBurned",
      defaultLabel: "Fuel Burned",
      value:
        flight.fuel_used_kg > 0
          ? `${Math.round(flight.fuel_used_kg).toLocaleString("en-US")}`
          : "—",
      unit: "kg",
      colorClass: "text-amber-400",
    },
    {
      labelKey: "myFlights.flightDuration",
      defaultLabel: "Flight Duration",
      value: formatDurationHoursMinutes(flight.flight_time_minutes),
      unit: "",
      sub:
        flight.flight_time_minutes > 0
          ? `${flight.flight_time_minutes} min`
          : undefined,
      colorClass: "text-sky-400",
    },
    {
      labelKey: "myFlights.operationalScore",
      defaultLabel: "Operational Score",
      value: scoreVal,
      unit: "/ 10",
      sub: tier.tier ? `${tier.tier} Tier` : undefined,
      colorClass: "text-[#e0a526]",
    },
  ];
}

export function getFlightFacts(flight: FlightLog): FlightFactItem[] {
  const flightNum = flight.flight_number || flight.callsign;
  const aircraftParts: string[] = [];
  if (flight.aircraft?.icao_code) aircraftParts.push(flight.aircraft.icao_code);
  if (flight.aircraft?.name) aircraftParts.push(`(${flight.aircraft.name})`);
  const aircraftDisplay = aircraftParts.join(" ") || "—";

  const depName = [
    flight.departure_airport?.icao,
    flight.departure_airport?.city || flight.departure_airport?.name,
  ]
    .filter(Boolean)
    .join(" - ");

  const arrName = [
    flight.arrival_airport?.icao,
    flight.arrival_airport?.city || flight.arrival_airport?.name,
  ]
    .filter(Boolean)
    .join(" - ");

  const facts: FlightFactItem[] = [
    {
      labelKey: "myFlights.factFlightNumber",
      defaultLabel: "Flight Number",
      value: flightNum,
    },
    {
      labelKey: "myFlights.factCallsign",
      defaultLabel: "Callsign",
      value: flight.callsign,
    },
    {
      labelKey: "myFlights.factRegistration",
      defaultLabel: "Registration",
      value: flight.aircraft?.registration || "—",
    },
    {
      labelKey: "myFlights.aircraftUsed",
      defaultLabel: "Aircraft",
      value: aircraftDisplay,
    },
    {
      labelKey: "myFlights.factDepartureAirport",
      defaultLabel: "Origin",
      value: depName || "—",
    },
    {
      labelKey: "myFlights.factArrivalAirport",
      defaultLabel: "Destination",
      value: arrName || "—",
    },
    {
      labelKey: "myFlights.factAirTime",
      defaultLabel: "Flight Time",
      value: formatDurationHoursMinutes(flight.flight_time_minutes),
    },
    {
      labelKey: "myFlights.factStatus",
      defaultLabel: "Status",
      value: flight.status || "—",
    },
  ];

  if (flight.departure_time) {
    facts.push({
      labelKey: "myFlights.factDepartureTime",
      defaultLabel: "Departure Time",
      value: flight.departure_time,
    });
  }

  if (flight.arrival_time) {
    facts.push({
      labelKey: "myFlights.factArrivalTime",
      defaultLabel: "Arrival Time",
      value: flight.arrival_time,
    });
  }

  if (flight.route && typeof flight.route === "string" && flight.route.trim()) {
    facts.push({
      labelKey: "myFlights.routeLabel",
      defaultLabel: "Route",
      value: flight.route.trim(),
    });
  }

  return facts;
}
