import { type ComponentProps, useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { AcarsDashboard } from "./acars-dashboard";
import en from "@/locales/en.json";
import pt from "@/locales/pt.json";
import es from "@/locales/es.json";
import fr from "@/locales/fr.json";

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false });
const props: ComponentProps<typeof AcarsDashboard> = {
  localMode: false, connectedAdapter: "SimConnect", connecting: false, flightState: "idle",
  booking: { callsign: "AZU123", departure_airport: { icao: "SBGR" }, arrival_airport: { icao: "SBRJ" } },
  activeFlightInfo: { callsign: "AZU123", departure: "SBGR", arrival: "SBRJ" },
  flightData: null, onGround: true, groundSpeed: 0, starting: false, ending: false,
  finishCooldown: 0, finishPending: null, volume: 40, refreshing: false, bookingError: false, error: null,
  onDismissError: vi.fn(), onConnect: vi.fn(), onDisconnect: vi.fn(), onRefresh: vi.fn(), onStart: vi.fn(),
  onStop: vi.fn(), onFinish: vi.fn(), onCancelFinish: vi.fn(), onVolumeChange: vi.fn(),
};
function view(overrides: Partial<typeof props> = {}) {
  return <I18nextProvider i18n={i18n}><AcarsDashboard {...props} {...overrides} /></I18nextProvider>;
}
afterEach(cleanup);

it("only enables start when the preflight requirements are met", () => {
  const { rerender } = render(view({ groundSpeed: 4 }));
  expect(screen.getByRole("button", { name: en["acars.startFlight"] })).toBeDisabled();
  rerender(view({ connectedAdapter: "" }));
  expect(screen.getByRole("button", { name: en["acars.startFlight"] })).toBeDisabled();
  rerender(view());
  fireEvent.click(screen.getByRole("button", { name: en["acars.startFlight"] }));
  expect(props.onStart).toHaveBeenCalledTimes(1);
});

it("keeps an active flight visible after disconnect and during finishing", () => {
  const { rerender } = render(view({ flightState: "active", connectedAdapter: "", finishCooldown: 25 }));
  expect(screen.getByText("SBGR")).toBeInTheDocument();
  expect(screen.getByText(en["acars.dashboard.connectionLost"])).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Finish Flight/ })).toBeDisabled();
  rerender(view({ flightState: "finishing", finishPending: 123 }));
  expect(screen.getByRole("status")).toHaveTextContent("123");
  expect(screen.getByRole("button", { name: en["acars.cancelFinish"] })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: en["acars.startFlight"] })).not.toBeInTheDocument();
});

it("mutes and restores the pilot's selected volume", () => {
  function Harness() {
    const [volume, setVolume] = useState(40);
    return view({ volume, onVolumeChange: setVolume });
  }
  render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: en["acars.dashboard.mute"] }));
  expect(screen.getAllByText("0%").length).toBeGreaterThanOrEqual(1);
  fireEvent.click(screen.getByRole("button", { name: en["acars.dashboard.unmute"] }));
  expect(screen.getByText("40%")).toBeInTheDocument();
  expect(screen.getByRole("slider", { name: en["acars.cabinAudio"] })).toHaveAttribute("aria-valuenow", "40");
});

it("keeps airline flight actions unavailable in local mode", () => {
  render(view({ localMode: true }));
  expect(screen.queryByRole("button", { name: en["acars.startFlight"] })).not.toBeInTheDocument();
  expect(screen.queryByText("AZU123")).not.toBeInTheDocument();
});

it("has translations and matching placeholders for all dashboard and clock labels", () => {
  const keys = Object.keys(en).filter((key) => key.startsWith("acars.dashboard.") || key.startsWith("clock."));
  for (const locale of [pt, es, fr]) for (const key of keys) {
    const value = locale[key as keyof typeof locale];
    expect(value, key).toBeTruthy();
    expect(value.match(/\{\{\w+\}\}/g) ?? []).toEqual(en[key as keyof typeof en].match(/\{\{\w+\}\}/g) ?? []);
  }
});
