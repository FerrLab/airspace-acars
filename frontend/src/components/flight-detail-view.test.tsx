import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { FlightDetailView } from "./flight-detail-view";

vi.mock("@/components/flights-map", () => ({
  FlightsMap: () => <div data-testid="flights-map-mock">Mocked FlightsMap</div>,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string | { defaultValue?: string }) => {
      if (typeof fallback === "string") return fallback;
      if (typeof fallback === "object" && fallback.defaultValue) return fallback.defaultValue;
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
  landing_rate_fpm: -508,
  fuel_used_kg: 3427,
  score: 92,
  created_at: "2026-10-08T14:36:00Z",
  departure_time: "2026-10-08T14:36:00Z",
  arrival_time: "2026-10-08T16:10:00Z",
};

describe("FlightDetailView Component", () => {
  const onBackMock = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders header with callsign, route, and status", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    expect(screen.getAllByText("SXB9404").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/SBGR/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/SBFI/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText("accepted").length).toBeGreaterThan(0);
  });

  it("triggers onBack when back button is clicked", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const backBtn = screen.getByRole("button", { name: /Voltar aos voos/i });
    fireEvent.click(backBtn);
    expect(onBackMock).toHaveBeenCalledTimes(1);
  });

  it("renders the 6 key flight KPI cards in hero strip", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    expect(screen.getByText("-508 ft/min")).toBeDefined(); // Touchdown rate
    expect(screen.getAllByText(/456/).length).toBeGreaterThan(0); // Distance
    expect(screen.getAllByText(/3,427/).length).toBeGreaterThan(0); // Fuel
    expect(screen.getAllByText("1h 20m").length).toBeGreaterThan(0); // Duration
  });

  it("switches to Landing tab and displays runway diagram & metrics", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const landingTabBtn = screen.getByRole("button", { name: /Pouso/i });
    fireEvent.click(landingTabBtn);

    expect(screen.getByText(/Diagrama da Pista/i)).toBeDefined();
    expect(screen.getByText(/Gate Estabilizado a 500 ft/i)).toBeDefined();
    expect(screen.getByText(/Vento no Toque/i)).toBeDefined();
  });

  it("switches to Takeoff tab and displays takeoff performance", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const takeoffTabBtn = screen.getByRole("button", { name: /Decolagem/i });
    fireEvent.click(takeoffTabBtn);

    expect(screen.getByText(/Desempenho de Decolagem/i)).toBeDefined();
    expect(screen.getByText(/Perfil de Subida/i)).toBeDefined();
  });

  it("switches to Telemetry tab and displays track controls", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const telemetryTabBtn = screen.getByRole("button", { name: /Telemetria/i });
    fireEvent.click(telemetryTabBtn);

    expect(screen.getByText(/Traçados Sincronizados/i)).toBeDefined();
    expect(screen.getByRole("button", { name: /Altitude/i })).toBeDefined();
    expect(screen.getByRole("button", { name: /Ground Speed/i })).toBeDefined();
  });

  it("switches to Score tab and displays composite score & categories", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const scoreTabBtn = screen.getByRole("button", { name: /Pontuação/i });
    fireEvent.click(scoreTabBtn);

    expect(screen.getByText(/Score Composto/i)).toBeDefined();
    expect(screen.getAllByText(/On-Time Performance/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Pontos de Voo/i)).toBeDefined();
  });

  it("switches to Log tab and displays audit entries", () => {
    render(<FlightDetailView flight={mockFlight} company="Voe Samba" onBack={onBackMock} />);

    const logTabBtn = screen.getByRole("button", { name: /Histórico/i });
    fireEvent.click(logTabBtn);

    expect(screen.getByText(/Registro de Atividade/i)).toBeDefined();
    expect(screen.getByText(/FDM Engine/i)).toBeDefined();
  });
});
