import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { formatUtcDate } from "@/lib/flight-format";

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
  // A line under the value: subKey is translated, sub is shown as it is.
  subKey?: string;
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

/**
 * landingRateBand names a touchdown the way the platform's flight page does
 * (LandingRateBand in FerrLab/airspace: up to 100 fpm butter, 200 good, 400
 * firm, harder than that hard), so the same landing gets the same word in
 * ACARS and on the web.
 */
export function landingRateBand(rate: number): {
  labelKey: string | null;
  colorClass: string;
} {
  const abs = Math.abs(rate);
  if (abs === 0) return { labelKey: null, colorClass: "text-muted-foreground" };
  if (abs <= 100) return { labelKey: "myFlights.landingBand.butter", colorClass: "text-emerald-400" };
  if (abs <= 200) return { labelKey: "myFlights.landingBand.good", colorClass: "text-sky-400" };
  if (abs <= 400) return { labelKey: "myFlights.landingBand.firm", colorClass: "text-amber-400" };
  return { labelKey: "myFlights.landingBand.hard", colorClass: "text-rose-400" };
}

/**
 * formatScore shows the server's score, which is the composite score × 10 and
 * so runs from 0 to 100, on the platform's 0–10 scale. A score of 0 is a
 * flight not scored yet.
 */
export function formatScore(score: number): string {
  return score > 0 ? (score / 10).toFixed(1) : "—";
}

/**
 * getFlightKPIs lists the headline numbers of a logged flight. locale is the
 * pilot's language, which numbers are grouped in.
 */
export function getFlightKPIs(flight: FlightLog, locale: string): FlightKPI[] {
  const band = landingRateBand(flight.landing_rate_fpm);

  return [
    {
      labelKey: "myFlights.touchdownRate",
      defaultLabel: "Touchdown Rate",
      value: flight.landing_rate_fpm
        ? `${Math.round(flight.landing_rate_fpm)}`
        : "—",
      unit: "ft/min",
      subKey: band.labelKey ?? undefined,
      colorClass: band.colorClass,
    },
    {
      labelKey: "myFlights.kpiTotalDistance",
      defaultLabel: "Total Distance",
      value: flight.distance_nm > 0 ? Math.round(flight.distance_nm).toLocaleString(locale) : "—",
      unit: "NM",
      sub:
        flight.distance_nm > 0
          ? `~${Math.round(flight.distance_nm * 1.852).toLocaleString(locale)} km`
          : undefined,
      colorClass: "text-foreground",
    },
    {
      labelKey: "myFlights.fuelBurned",
      defaultLabel: "Fuel Burned",
      value:
        flight.fuel_used_kg > 0
          ? Math.round(flight.fuel_used_kg).toLocaleString(locale)
          : "—",
      unit: "kg",
      colorClass: "text-amber-400",
    },
    {
      labelKey: "myFlights.flightDuration",
      defaultLabel: "Flight Duration",
      value: formatDurationHoursMinutes(flight.flight_time_minutes),
      unit: "",
      colorClass: "text-sky-400",
    },
    {
      labelKey: "myFlights.operationalScore",
      defaultLabel: "Operational Score",
      value: formatScore(flight.score),
      unit: "/ 10",
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

  // The status is not a fact here: the header shows it as the translated,
  // coloured badge the logbook list uses.
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
  ];

  if (flight.departure_time) {
    facts.push({
      labelKey: "myFlights.factDepartureTime",
      defaultLabel: "Departure Time",
      value: formatUtcDate(flight.departure_time),
    });
  }

  if (flight.arrival_time) {
    facts.push({
      labelKey: "myFlights.factArrivalTime",
      defaultLabel: "Arrival Time",
      value: formatUtcDate(flight.arrival_time),
    });
  }

  return facts;
}
