import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { FlightDetailView } from "./flight-detail-view";
import en from "../locales/en.json";

vi.mock("@/components/flights-map", () => ({
  FlightsMap: () => <div data-testid="flights-map-mock">Mocked FlightsMap</div>,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string | { defaultValue?: string }) => {
      const enDict = en as Record<string, string>;
      if (enDict[key]) return enDict[key];
      if (typeof fallback === "string") return fallback;
      if (typeof fallback === "object" && fallback.defaultValue)
        return fallback.defaultValue;
      return key;
    },
  }),
}));

const mockFlight: FlightLog = {
  id: "880",
  callsign: "SXB9404",
  flight_number: "4J9404",
  status: "accepted",
  departure_airport: {
    icao: "SBGR",
    name: "Guarulhos",
    city: "São Paulo",
    latitude: -23.4356,
    longitude: -46.4731,
  },
  arrival_airport: {
    icao: "SBFI",
    name: "Cataratas",
    city: "Foz do Iguaçu",
    latitude: -25.5958,
    longitude: -54.4875,
  },
  aircraft: {
    registration: "PR-SBG",
    name: "Airbus A320ceo IAE",
    icao_code: "A320",
  },
  flight_time_minutes: 80,
  distance_nm: 456,
  landing_rate_fpm: -142,
  fuel_used_kg: 3427,
  score: 92,
  created_at: "2026-10-08T14:36:00Z",
  departure_time: "2026-10-08T14:36:00Z",
  arrival_time: "2026-10-08T16:10:00Z",
  route: "BITIS UZ25 KASUK",
};

describe("FlightDetailView Component", () => {
  const onBackMock = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders header with callsign, route, and status", () => {
    render(
      <FlightDetailView
        flight={mockFlight}
        company="Voe Samba"
        onBack={onBackMock}
      />
    );

    expect(screen.getAllByText("SXB9404").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/SBGR/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/SBFI/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText("accepted").length).toBeGreaterThan(0);
  });

  it("triggers onBack when back button is clicked using en.json key", () => {
    render(
      <FlightDetailView
        flight={mockFlight}
        company="Voe Samba"
        onBack={onBackMock}
      />
    );

    const backBtn = screen.getByRole("button", {
      name: new RegExp(en["myFlights.backToFlights"], "i"),
    });
    fireEvent.click(backBtn);
    expect(onBackMock).toHaveBeenCalledTimes(1);
  });

  it("renders real flight KPI cards in hero strip using en.json keys", () => {
    render(
      <FlightDetailView
        flight={mockFlight}
        company="Voe Samba"
        onBack={onBackMock}
      />
    );

    expect(screen.getByText(en["myFlights.touchdownRate"])).toBeDefined();
    expect(screen.getByText("-142 ft/min")).toBeDefined();

    expect(screen.getByText(en["myFlights.kpiTotalDistance"])).toBeDefined();
    expect(screen.getAllByText("456 NM").length).toBeGreaterThan(0);

    expect(screen.getByText(en["myFlights.fuelBurned"])).toBeDefined();
    expect(screen.getByText("3,427 kg")).toBeDefined();

    expect(screen.getByText(en["myFlights.flightDuration"])).toBeDefined();
    expect(screen.getAllByText("1h 20m").length).toBeGreaterThan(0);

    expect(screen.getByText(en["myFlights.operationalScore"])).toBeDefined();
    expect(screen.getByText("9.2 / 10")).toBeDefined();
  });

  it("renders flight facts summary with real data and route", () => {
    render(
      <FlightDetailView
        flight={mockFlight}
        company="Voe Samba"
        onBack={onBackMock}
      />
    );

    expect(screen.getByText(en["myFlights.flightSummary"])).toBeDefined();
    expect(screen.getByText(en["myFlights.factRegistration"])).toBeDefined();
    expect(screen.getByText("PR-SBG")).toBeDefined();

    expect(screen.getByText(en["myFlights.routeLabel"])).toBeDefined();
    expect(screen.getByText("BITIS UZ25 KASUK")).toBeDefined();

    expect(screen.getByTestId("flights-map-mock")).toBeDefined();
  });
});
