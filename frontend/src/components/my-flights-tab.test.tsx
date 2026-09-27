import { act, fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { FlightLogService } from "../../bindings/airspace-acars";
import { MyFlightsTab } from "./my-flights-tab";
import en from "@/locales/en.json";

// Mock FlightsMap so tests don't require full Leaflet DOM Canvas/SVG dimensions
vi.mock("@/components/flights-map", () => ({
  FlightsMap: ({ flights, selectedFlightId, onSelectFlight }: any) => (
    <div data-testid="flights-map" data-selected={selectedFlightId}>
      <span>Mapped Flights: {flights.length}</span>
      {flights.map((f: any) => (
        <button key={f.id} onClick={() => onSelectFlight(f.id)}>
          Focus {f.callsign}
        </button>
      ))}
    </div>
  ),
}));

const auth = vi.hoisted(() => ({
  tenant: { id: "test-va", domain: "va.example", name: "Star VA" },
  tokenSynced: true,
}));
vi.mock("@/context/auth-context", () => ({ useAuth: () => auth }));

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

const mockFlightLogs = [
  {
    id: "flt-1",
    callsign: "GLO1234",
    flight_number: "1234",
    status: "accepted",
    departure_airport: {
      icao: "SBGR",
      name: "Guarulhos",
      city: "Sao Paulo",
      latitude: -23.435,
      longitude: -46.473,
    },
    arrival_airport: {
      icao: "SBRJ",
      name: "Santos Dumont",
      city: "Rio de Janeiro",
      latitude: -22.91,
      longitude: -43.163,
    },
    aircraft: {
      registration: "PR-GXZ",
      icao_code: "B738",
    },
    flight_time_minutes: 55,
    distance_nm: 196,
    landing_rate_fpm: -142,
    fuel_used_kg: 1450,
    score: 98,
    created_at: "2026-09-20T14:30:00Z",
  },
  {
    id: "flt-2",
    callsign: "GLO1235",
    flight_number: "1235",
    status: "accepted",
    departure_airport: {
      icao: "SBRJ",
      name: "Santos Dumont",
      city: "Rio de Janeiro",
      latitude: -22.91,
      longitude: -43.163,
    },
    arrival_airport: {
      icao: "SBBR",
      name: "Brasilia",
      city: "Brasilia",
      latitude: -15.869,
      longitude: -47.92,
    },
    aircraft: {
      registration: "PR-XYZ",
      icao_code: "A320",
    },
    flight_time_minutes: 95,
    distance_nm: 480,
    landing_rate_fpm: -190,
    fuel_used_kg: 2100,
    score: 95,
    created_at: "2026-09-21T18:00:00Z",
  },
];

const mockPilotStats = {
  pilot_id: "1",
  name: "Captain John Doe",
  callsign: "GLO101",
  rank: "Commander",
  total_flights: 2,
  total_hours: 2.5,
  avg_landing_rate: -166,
  total_distance_nm: 676,
  points: 450,
};

beforeEach(() => {
  auth.tenant = { id: "test-va", domain: "va.example", name: "Star VA" };
  vi.spyOn(FlightLogService, "GetMyFlights").mockResolvedValue({
    status: "ok",
    pilot: mockPilotStats as any,
    flights: mockFlightLogs as any,
    current_page: 1,
    last_page: 1,
    total: 2,
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("renders pilot statistics and flight records", async () => {
  render(view());

  expect(await screen.findByText("GLO101")).toBeInTheDocument();
  expect(screen.getByText("Commander")).toBeInTheDocument();
  expect(screen.getByText("2.5")).toBeInTheDocument();
  expect(screen.getByText("-166")).toBeInTheDocument();
  expect(screen.getByText("676")).toBeInTheDocument();

  expect(screen.getAllByText("GLO1234").length).toBeGreaterThan(0);
  expect(screen.getAllByText("GLO1235").length).toBeGreaterThan(0);
  expect(screen.getByTestId("flights-map")).toBeInTheDocument();
});

it("filters flight records with search input", async () => {
  render(view());
  await screen.findByText("GLO101");

  const searchInput = screen.getByPlaceholderText(en["myFlights.searchPlaceholder"]);
  await act(async () => {
    fireEvent.change(searchInput, { target: { value: "Brasilia" } });
  });

  // Table should show flight #1235 and not #1234
  expect(screen.getByText("#1235")).toBeInTheDocument();
  expect(screen.queryByText("#1234")).not.toBeInTheDocument();
});

it("shows local mode banner when localMode is active", async () => {
  render(view(true));

  expect(screen.getByText(en["myFlights.localMode"])).toBeInTheDocument();
  expect(FlightLogService.GetMyFlights).not.toHaveBeenCalled();
});

it("handles unauthorized api response gracefully without showing error banner", async () => {
  vi.spyOn(FlightLogService, "GetMyFlights").mockResolvedValueOnce({
    status: "accessDenied",
    pilot: {
      pilot_id: "1",
      name: "Captain John Doe",
      callsign: "GLO101",
      rank: "Commander",
      total_flights: 0,
      total_hours: 0,
      avg_landing_rate: 0,
      total_distance_nm: 0,
      points: 0,
    } as any,
    flights: [],
    current_page: 1,
    last_page: 1,
    total: 0,
  });

  render(view());
  expect(await screen.findByText("GLO101")).toBeInTheDocument();
  expect(screen.queryByText(en["myFlights.accessDenied"])).not.toBeInTheDocument();
  expect(screen.getByText("Nenhum voo encontrado")).toBeInTheDocument();
});
