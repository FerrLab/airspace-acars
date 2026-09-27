import { act, fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { FlightLogService } from "../../bindings/airspace-acars";
import type {
  FlightLog,
  PilotSummaryStats,
} from "../../bindings/airspace-acars/internal/domain/models";
import { MyFlightsTab } from "./my-flights-tab";
import en from "@/locales/en.json";

// Mock FlightsMap so tests don't require full Leaflet DOM Canvas/SVG dimensions
vi.mock("@/components/flights-map", () => ({
  FlightsMap: ({ flights, selectedFlightId, overlayContent }: any) => (
    <div data-testid="flights-map" data-selected={selectedFlightId}>
      <span>Mapped Flights: {flights?.length || 0}</span>
      {overlayContent}
    </div>
  ),
}));

const auth = vi.hoisted(() => ({
  tenant: { id: "test-va", domain: "va.example", name: "Eagle" },
  tokenSynced: true,
}));
vi.mock("@/context/auth-context", () => ({ useAuth: () => auth }));

const sampleFlights: FlightLog[] = [
  {
    id: "flt-01",
    callsign: "SXB1265",
    flight_number: "1265",
    status: "accepted",
    departure_airport: {
      icao: "SBCA",
      name: "Coronel Adalberto Mendes",
      city: "Cascavel",
      country: "Brasil",
      latitude: -24.9539,
      longitude: -53.5008,
    },
    arrival_airport: {
      icao: "SBGR",
      name: "Guarulhos Intl",
      city: "São Paulo",
      country: "Brasil",
      latitude: -23.4356,
      longitude: -46.4731,
    },
    aircraft: { registration: "PR-GXA", name: "Boeing 737 MAX 8", icao_code: "B38M" },
    flight_time_minutes: 82,
    distance_nm: 440,
    landing_rate_fpm: -128,
    fuel_used_kg: 2150,
    score: 98,
    departure_time: "2026-09-20T14:26:00Z",
    created_at: "2026-09-20T14:26:00Z",
  },
  {
    id: "flt-02",
    callsign: "SXB1266",
    flight_number: "1266",
    status: "accepted",
    departure_airport: {
      icao: "SBGR",
      name: "Guarulhos Intl",
      city: "São Paulo",
      country: "Brasil",
      latitude: -23.4356,
      longitude: -46.4731,
    },
    arrival_airport: {
      icao: "SBCA",
      name: "Coronel Adalberto Mendes",
      city: "Cascavel",
      country: "Brasil",
      latitude: -24.9539,
      longitude: -53.5008,
    },
    aircraft: { registration: "PR-GXB", name: "Boeing 737 MAX 8", icao_code: "B38M" },
    flight_time_minutes: 85,
    distance_nm: 440,
    landing_rate_fpm: -115,
    fuel_used_kg: 2200,
    score: 100,
    departure_time: "2026-09-19T02:34:00Z",
    created_at: "2026-09-19T02:34:00Z",
  },
  {
    id: "flt-03",
    callsign: "SXB1021",
    flight_number: "1021",
    status: "accepted",
    departure_airport: {
      icao: "SBRJ",
      name: "Santos Dumont",
      city: "Rio de Janeiro",
      country: "Brasil",
      latitude: -22.9103,
      longitude: -43.1631,
    },
    arrival_airport: {
      icao: "SBSP",
      name: "Congonhas",
      city: "São Paulo",
      country: "Brasil",
      latitude: -23.6261,
      longitude: -46.6564,
    },
    aircraft: { registration: "PR-GXE", name: "Boeing 737 MAX 8", icao_code: "B38M" },
    flight_time_minutes: 42,
    distance_nm: 198,
    landing_rate_fpm: -130,
    fuel_used_kg: 1200,
    score: 99,
    departure_time: "2026-09-18T18:00:00Z",
    created_at: "2026-09-18T18:00:00Z",
  },
  {
    id: "flt-04",
    callsign: "SXB8105",
    flight_number: "8105",
    status: "pending",
    departure_airport: {
      icao: "SBGR",
      name: "Guarulhos Intl",
      city: "São Paulo",
      country: "Brasil",
      latitude: -23.4356,
      longitude: -46.4731,
    },
    arrival_airport: {
      icao: "KMIA",
      name: "Miami International",
      city: "Miami",
      country: "EUA",
      latitude: 25.7933,
      longitude: -80.2906,
    },
    aircraft: { registration: "PR-GXF", name: "Boeing 777-300ER", icao_code: "B77W" },
    flight_time_minutes: 510,
    distance_nm: 3600,
    landing_rate_fpm: -155,
    fuel_used_kg: 48000,
    score: 92,
    departure_time: "2026-09-17T21:10:00Z",
    created_at: "2026-09-17T21:10:00Z",
  },
];

const samplePilotStats: PilotSummaryStats = {
  pilot_id: "PIL-100",
  name: "John Doe",
  callsign: "SXB101",
  rank: "Captain",
  rank_image_url: "",
  total_flights: 50,
  total_hours: 84.5,
  avg_landing_rate: -162,
  total_distance_nm: 29420,
  points: 4850,
};

const i18n = createInstance();
await i18n.init({
  lng: "en",
  resources: { en: { translation: en } },
  keySeparator: false,
  interpolation: { escapeValue: false },
});

function view(localMode = false) {
  return (
    <I18nextProvider i18n={i18n}>
      <MyFlightsTab localMode={localMode} />
    </I18nextProvider>
  );
}

beforeEach(() => {
  auth.tenant = { id: "test-va", domain: "va.example", name: "Eagle" };
  vi.spyOn(FlightLogService, "GetMyFlights").mockResolvedValue({
    status: "ok",
    pilot: samplePilotStats,
    flights: sampleFlights,
    current_page: 1,
    last_page: 1,
    total: sampleFlights.length,
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("renders flight table and pilot stats strictly adhering to the Flux UI design system", async () => {
  render(view());

  expect(screen.getByText("My Flights")).toBeInTheDocument();
  expect(screen.getByText("Total Flights")).toBeInTheDocument();
  expect(screen.getByText("Flight Hours")).toBeInTheDocument();
  expect(screen.getByText("Avg Landing")).toBeInTheDocument();
  expect(screen.getByText("Total Distance")).toBeInTheDocument();

  // Flux UI Table columns strictly reflecting API fields
  expect(screen.getByText("Flight")).toBeInTheDocument();
  expect(screen.getByText("Origin")).toBeInTheDocument();
  expect(screen.getByText("Destination")).toBeInTheDocument();
  expect(screen.getByText("Departure (UTC)")).toBeInTheDocument();
  expect(screen.getByText("Aircraft")).toBeInTheDocument();
  expect(screen.getByText("Duration")).toBeInTheDocument();
  expect(screen.getByText("Distance")).toBeInTheDocument();
  expect(screen.getByText("Landing")).toBeInTheDocument();
  expect(screen.getByText("Score")).toBeInTheDocument();
  expect(screen.getByText("Status")).toBeInTheDocument();
  expect(screen.getByText("Action")).toBeInTheDocument();

  // First flight row SXB1265
  expect(await screen.findByText("SXB1265")).toBeInTheDocument();
  expect(screen.getAllByText("SBCA").length).toBeGreaterThan(0);
  expect(screen.getAllByText("SBGR").length).toBeGreaterThan(0);
  expect(screen.getByText("-128 ft/min")).toBeInTheDocument();
  expect(screen.getAllByText("98 pts").length).toBeGreaterThan(0);

  // Second flight SXB1266
  expect(screen.getByText("SXB1266")).toBeInTheDocument();
  expect(screen.getAllByText("-115 ft/min").length).toBeGreaterThan(0);
});

it("filters flight records with search input", async () => {
  render(view());
  await screen.findByText("SXB1265");

  const searchInput = screen.getByPlaceholderText(en["myFlights.searchPlaceholder"]);
  await act(async () => {
    fireEvent.change(searchInput, { target: { value: "SXB1021" } });
  });

  expect(screen.getByText("SXB1021")).toBeInTheDocument();
  expect(screen.queryByText("SXB1265")).not.toBeInTheDocument();
});

it("clicking the eye action button opens full-screen flight detail view with map", async () => {
  render(view());
  await screen.findByText("SXB1265");

  // Find eye action buttons
  const eyeButtons = screen.getAllByTitle(en["myFlights.viewFlightDetails"]);
  expect(eyeButtons.length).toBeGreaterThan(0);

  // Click eye button on first row (SXB1265)
  await act(async () => {
    fireEvent.click(eyeButtons[0]);
  });

  // Full-screen detail view is shown with route map and metrics
  expect(screen.getByText("Operational Audit Report (vOCC)")).toBeInTheDocument();
  expect(screen.getByText("Flight Duration")).toBeInTheDocument();
  expect(screen.getByText("Touchdown Rate")).toBeInTheDocument();
  expect(screen.getAllByText("-128 ft/min").length).toBeGreaterThan(0);
  expect(screen.getByText("Fuel Consumed")).toBeInTheDocument();
  expect(screen.getByText("Aircraft Used")).toBeInTheDocument();
  expect(screen.getByText("Operational Score")).toBeInTheDocument();
  expect(screen.getByTestId("flights-map")).toBeInTheDocument();

  // Click Back to flights
  const backBtn = screen.getByText("Back to flights");
  await act(async () => {
    fireEvent.click(backBtn);
  });

  // Returns to the table
  expect(screen.queryByText("Operational Audit Report (vOCC)")).not.toBeInTheDocument();
  expect(screen.getByText("SXB1265")).toBeInTheDocument();
});

it("filters by status using segmented buttons", async () => {
  render(view());
  await screen.findByText("SXB1265");

  // Click on Pending filter button
  const pendingBtn = screen.getByRole("button", { name: "Pending" });
  await act(async () => {
    fireEvent.click(pendingBtn);
  });

  // SXB8105 is the pending flight in sample dataset
  expect(screen.getByText("SXB8105")).toBeInTheDocument();
  expect(screen.queryByText("SXB1265")).not.toBeInTheDocument();
});

it("renders local mode message when localMode is true", async () => {
  render(view(true));
  expect(await screen.findByText(en["myFlights.localMode"])).toBeInTheDocument();
});
