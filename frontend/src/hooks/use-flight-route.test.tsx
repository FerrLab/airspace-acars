import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { FlightService } from "../../bindings/airspace-acars";
import { ROUTE_REFRESH_MS, useFlightRoute } from "./use-flight-route";

type Route = Awaited<ReturnType<typeof FlightService.GetFlightRoute>>;

const sbgrToSbrj: Route = {
  departure: { icao: "SBGR", latitude: -23.435556, longitude: -46.473056 },
  arrival: { icao: "SBRJ", latitude: -22.910278, longitude: -43.163056 },
};

function deferred() {
  let resolve: (r: Route) => void = () => {};
  const promise = new Promise<Route>((r) => (resolve = r));
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("takes the route's coordinates from the backend", async () => {
  vi.spyOn(FlightService, "GetFlightRoute").mockResolvedValue(sbgrToSbrj);

  const { result } = renderHook(() => useFlightRoute("idle", "1:SBGR:SBRJ"));

  await waitFor(() => expect(result.current.arrival).toEqual({ lat: -22.910278, lon: -43.163056 }));
  expect(result.current.departure).toEqual({ lat: -23.435556, lon: -46.473056 });
});

// The backend reports an airport it cannot place with zero coordinates; that
// must read as unknown, not as a point in the Gulf of Guinea.
it("treats an airport without coordinates as unknown", async () => {
  vi.spyOn(FlightService, "GetFlightRoute").mockResolvedValue({
    departure: { icao: "SWYY", latitude: 0, longitude: 0 },
    arrival: sbgrToSbrj!.arrival,
  });

  const { result } = renderHook(() => useFlightRoute("idle", "1:SWYY:SBRJ"));

  await waitFor(() => expect(result.current.arrival).not.toBeNull());
  expect(result.current.departure).toBeNull();
});

// A new booking must never be shown with the previous one's distance: the old
// route goes as soon as the booking changes, and a slow reply for it that
// lands afterwards is dropped.
it("drops the previous booking's route when the booking changes", async () => {
  const first = deferred();
  const second = deferred();
  vi.spyOn(FlightService, "GetFlightRoute")
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(second.promise);

  const { result, rerender } = renderHook(({ key }) => useFlightRoute("idle", key), {
    initialProps: { key: "1:SBGR:SBRJ" as string | null },
  });
  rerender({ key: "2:SBGR:KDFW" });
  await act(async () => first.resolve(sbgrToSbrj));

  expect(result.current).toEqual({ departure: null, arrival: null });

  rerender({ key: null });
  await act(async () => second.resolve(sbgrToSbrj));
  expect(result.current).toEqual({ departure: null, arrival: null });
});

// During a flight the backend learns the booking's departure coordinates and
// any diversion from its own booking refresh, so the route is read again.
it("reads the route again while the flight lasts", async () => {
  vi.useFakeTimers();
  const spy = vi.spyOn(FlightService, "GetFlightRoute").mockResolvedValue(sbgrToSbrj);

  const { rerender } = renderHook(({ state }) => useFlightRoute(state, null), {
    initialProps: { state: "active" as "idle" | "active" | "finishing" },
  });
  expect(spy).toHaveBeenCalledTimes(1);

  await act(async () => vi.advanceTimersByTime(ROUTE_REFRESH_MS));
  expect(spy).toHaveBeenCalledTimes(2);

  // Finishing is the same flight: the route is kept, not reloaded.
  rerender({ state: "finishing" });
  expect(spy).toHaveBeenCalledTimes(2);
});
