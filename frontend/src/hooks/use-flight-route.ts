import { useEffect, useState } from "react";
import { FlightService } from "../../bindings/airspace-acars";
import type { Coordinates } from "@/lib/flight-progress";

export interface FlightRouteCoords {
  departure: Coordinates | null;
  arrival: Coordinates | null;
}

const NO_ROUTE: FlightRouteCoords = { departure: null, arrival: null };

// How often the route is read again during a flight. The backend re-reads the
// booking every 30 s, which is when a diversion or the booking's departure
// coordinates reach it; reading it here is local, with no request to the
// tenant.
export const ROUTE_REFRESH_MS = 15_000;

interface RouteAirport {
  latitude?: number;
  longitude?: number;
}

function coordinatesOf(airport: RouteAirport | null | undefined): Coordinates | null {
  const lat = airport?.latitude;
  const lon = airport?.longitude;
  if (typeof lat !== "number" || typeof lon !== "number" || (lat === 0 && lon === 0)) return null;
  return { lat, lon };
}

/**
 * useFlightRoute keeps the coordinates of the flight's departure and arrival
 * for the progress bar. The backend takes them from the booking, or from its
 * built-in airport table when the booking has none, so the webview never reads
 * the server's airport shapes.
 *
 * bookingKey names the booked route while idle (null when there is no
 * booking); a new key clears the old route at once, so one booking's distance
 * is never shown for another.
 */
export function useFlightRoute(
  flightState: "idle" | "active" | "finishing",
  bookingKey: string | null,
): FlightRouteCoords {
  const [route, setRoute] = useState<FlightRouteCoords>(NO_ROUTE);
  const inFlight = flightState !== "idle";
  const key = inFlight ? "flight" : bookingKey;

  useEffect(() => {
    setRoute(NO_ROUTE);
    if (key === null) return;
    let cancelled = false;
    const load = () => {
      FlightService.GetFlightRoute()
        .then((r: { departure?: RouteAirport | null; arrival?: RouteAirport | null } | null) => {
          if (cancelled) return;
          setRoute({ departure: coordinatesOf(r?.departure), arrival: coordinatesOf(r?.arrival) });
        })
        .catch(() => {
          if (!cancelled) setRoute(NO_ROUTE);
        });
    };
    load();
    const timer = inFlight ? window.setInterval(load, ROUTE_REFRESH_MS) : undefined;
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [key, inFlight]);

  return route;
}
