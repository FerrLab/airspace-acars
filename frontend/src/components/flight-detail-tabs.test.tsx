import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FlightLogService } from "../../bindings/airspace-acars";
import type { FlightDetail, FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { FlightDetailTabs } from "./flight-detail-tabs";
import en from "../locales/en.json";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: string | Record<string, unknown>) => {
      const text = (en as Record<string, string>)[key] ?? (typeof values === "string" ? values : key);
      if (!values || typeof values === "string") return text;
      return text.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(values[name] ?? ""));
    },
    i18n: { language: "en" },
  }),
}));

const msg = (key: string, values: Record<string, string | number> = {}) =>
  (en as Record<string, string>)[key].replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(values[name]));

const flight = {
  id: "880",
  callsign: "SXB9404",
  status: "acceptedSystem",
  departure_airport: { icao: "SBGR", latitude: -23.4, longitude: -46.4 },
  arrival_airport: { icao: "SBFI", latitude: -25.6, longitude: -54.5 },
  landing_rate_fpm: -142,
} as FlightLog;

function recorded(overrides: Partial<FlightDetail> = {}): FlightDetail {
  return {
    id: "880",
    landing_g_force: 1.18,
    landing: {
      touchdown_distance_m: 420,
      touchdown_offset_m: -1.5,
      touchdown_ground_speed: 132,
      touchdown_indicated_airspeed: 138,
      runway_used_m: 1150,
      runway_used_percent: 87.5,
      runway: { designator: "15", magnetic_heading: 150, length_m: 2195, width_m: 45 },
      wind: { direction_degrees: 150, speed_knots: 10, headwind_knots: 10, crosswind_knots: 0 },
      stabilized_approach: {
        gate_height_ft: 1000,
        passed: false,
        criteria: null,
        failures: ["Approach unstabilized at 1000 ft above field: flaps (40%) below the required minimum (70%)"],
      },
    },
    takeoff: null,
    score_breakdown: {
      score: 8.6,
      contributing: ["safety"],
      categories: [
        { key: "safety", score: 8, weight: 1, available: true, formula: "10 minus 2 per violation", inputs: [{ label: "FDM violations", value: "1" }] },
      ],
    },
    events: [{ id: "77", type: "violation", event: "Approach unstabilized", happened_at: "2026-10-08T16:05:00Z", duration_seconds: null }],
    points: { total: -50, entries: [{ id: "9", source: "fdm", reason: "Unstable approach", points: -50 }] },
    ...overrides,
  } as FlightDetail;
}

function showDetail(detail: FlightDetail | null, status = "ok") {
  return vi.spyOn(FlightLogService, "GetFlightDetail").mockResolvedValue({ status, data: detail } as never);
}

function renderTabs() {
  return render(<FlightDetailTabs flight={flight} overview={<p>overview content</p>} />);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// A tab exists only for data the platform recorded, so the pilot is never
// shown placeholder numbers as if they had been measured.
describe("FlightDetailTabs", () => {
  it("keeps the view at Overview when the airline cannot serve the detail", async () => {
    const detail = showDetail(null, "unavailable");
    renderTabs();

    await waitFor(() => expect(detail).toHaveBeenCalledWith("880"));
    expect(screen.getByText("overview content")).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });

  it("hides Landing, Takeoff and Score when nothing was recorded", async () => {
    showDetail(recorded({ landing: null, takeoff: null, score_breakdown: { score: null, contributing: [], categories: [] } as never }));
    renderTabs();

    expect(await screen.findByRole("tab", { name: en["myFlights.tabs.telemetry"] })).toBeInTheDocument();
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([en["myFlights.tabs.overview"], en["myFlights.tabs.telemetry"]]);
  });

  it("shows the touchdown the platform measured, including a failed stabilized gate", async () => {
    showDetail(recorded());
    const user = userEvent.setup();
    renderTabs();

    await user.click(await screen.findByRole("tab", { name: en["myFlights.tabs.landing"] }));

    expect(screen.getByText(msg("myFlights.units.metres", { value: 420 }))).toBeInTheDocument();
    expect(screen.getByText(msg("myFlights.runway.leftOfCentreline", { value: "1.5" }))).toBeInTheDocument();
    expect(screen.getByText(msg("myFlights.wind.headwind", { value: "10.0" }))).toBeInTheDocument();
    expect(screen.getByText(msg("myFlights.gate.title", { height: 1000 }))).toBeInTheDocument();
    expect(screen.getByText(en["myFlights.gate.fail"])).toBeInTheDocument();
    expect(screen.getByText(/flaps \(40%\) below the required minimum/)).toBeInTheDocument();
    expect(screen.getByRole("img", { name: msg("myFlights.runway.diagramLabel", { designator: "15" }) })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: en["myFlights.tabs.takeoff"] })).not.toBeInTheDocument();
  });

  it("explains the score with the recorded events and points", async () => {
    showDetail(recorded());
    const user = userEvent.setup();
    renderTabs();

    await user.click(await screen.findByRole("tab", { name: en["myFlights.tabs.score"] }));

    expect(screen.getByText("8.60")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: en["myFlights.score.category.safety"] })).toHaveAttribute("aria-valuenow", "8");
    expect(screen.getByText("10 minus 2 per violation")).toBeInTheDocument();
    expect(screen.getByText("Approach unstabilized")).toBeInTheDocument();
    expect(screen.getByText("Unstable approach")).toBeInTheDocument();
    expect(screen.getAllByText("-50")).toHaveLength(2);
  });

  it("fetches the track only when Telemetry is opened", async () => {
    showDetail(recorded());
    const track = vi.spyOn(FlightLogService, "GetFlightTrack").mockResolvedValue({
      status: "ok",
      points: [
        { sent_at: "2026-10-08T14:40:00Z", phase: "Climb", lat: -23.4, lng: -46.4, altitude: 2000, altitude_agl: null, indicated_airspeed: 200, ground_speed: 210, vertical_speed: 1800, heading: 150 },
        { sent_at: "2026-10-08T15:10:00Z", phase: "Cruise", lat: -24.4, lng: -50.4, altitude: 36000, altitude_agl: null, indicated_airspeed: 280, ground_speed: 450, vertical_speed: 0, heading: 260 },
      ],
    } as never);
    const user = userEvent.setup();
    renderTabs();

    await screen.findByRole("tab", { name: en["myFlights.tabs.landing"] });
    expect(track).not.toHaveBeenCalled();

    await user.click(screen.getByRole("tab", { name: en["myFlights.tabs.telemetry"] }));

    expect(await screen.findByRole("img", { name: en["myFlights.telemetry.chartLabel"] })).toBeInTheDocument();
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith("880");
  });

  it("says so when the flight has no recorded positions", async () => {
    showDetail(recorded());
    vi.spyOn(FlightLogService, "GetFlightTrack").mockResolvedValue({ status: "ok", points: [] } as never);
    const user = userEvent.setup();
    renderTabs();

    await user.click(await screen.findByRole("tab", { name: en["myFlights.tabs.telemetry"] }));

    expect(await screen.findByText(en["myFlights.telemetry.none"])).toBeInTheDocument();
  });

  it("does not let a late reply for the previous flight land on the next one", async () => {
    let answerFirst: (value: unknown) => void = () => {};
    vi.spyOn(FlightLogService, "GetFlightDetail")
      .mockImplementationOnce(() => new Promise((resolve) => (answerFirst = resolve)) as never)
      .mockResolvedValueOnce({ status: "ok", data: recorded({ id: "881", landing: null }) } as never);

    const { rerender } = renderTabs();
    rerender(<FlightDetailTabs flight={{ ...flight, id: "881" } as FlightLog} overview={<p>overview content</p>} />);
    await screen.findByRole("tab", { name: en["myFlights.tabs.telemetry"] });

    await act(async () => answerFirst({ status: "ok", data: recorded() }));

    expect(screen.queryByRole("tab", { name: en["myFlights.tabs.landing"] })).not.toBeInTheDocument();
  });
});
