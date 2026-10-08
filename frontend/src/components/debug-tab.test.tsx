import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { DebugService, FlightDataService } from "../../bindings/airspace-acars";
import { EMPTY_SNAPSHOT, type DebugSnapshot } from "@/lib/debug-snapshot";
import en from "@/locales/en.json";
import { DebugTab } from "./debug-tab";

vi.mock("@/hooks/use-flight-data", () => ({ useFlightData: () => ({ flightData: null }) }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

const sentAt = new Date(Date.now() - 4000).toISOString();
const atGate7: DebugSnapshot = {
  ground: { airport: "SBRF", runways: 1, stands: 41, runway: "", stand: "7", loadedAt: sentAt, checkedAt: sentAt, lastError: "" },
  report: { json: '{\n  "stand": {\n    "name": "7"\n  }\n}', at: sentAt, outcome: "preview", batchSize: 1 },
  profiles: ["fenix-a32x"],
};

function view() {
  return <I18nextProvider i18n={i18n}><DebugTab /></I18nextProvider>;
}

beforeEach(() => {
  vi.spyOn(DebugService, "GetDebugSnapshot").mockResolvedValue(atGate7);
  vi.spyOn(FlightDataService, "IsConnected").mockResolvedValue(true);
  vi.spyOn(FlightDataService, "ConnectedAdapter").mockResolvedValue("SimConnect");
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("shows the airport and the stand the backend places the aircraft on", async () => {
  render(view());
  expect(await screen.findByRole("row", { name: /Stand\s*7/ })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: /Airport\s*SBRF · 1 runways, 41 stands/ })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: /Runway\s*—/ })).toBeInTheDocument();
});

// Before any lookup the screen claims nothing about airports: "no airport
// nearby" is only an answer once a lookup has given it.
it("makes no airport claim before any lookup has run", async () => {
  vi.mocked(DebugService.GetDebugSnapshot).mockResolvedValue(EMPTY_SNAPSHOT);
  render(view());
  expect(await screen.findByText(en["debug.ground.notLoaded"])).toBeInTheDocument();
  expect(screen.getByRole("row", { name: /Airport\s*—/ })).toBeInTheDocument();
  expect(screen.queryByText(en["debug.ground.noAirport"])).not.toBeInTheDocument();
});

it("says no airport was found once a lookup found none", async () => {
  vi.mocked(DebugService.GetDebugSnapshot).mockResolvedValue({
    ...EMPTY_SNAPSHOT,
    ground: { ...EMPTY_SNAPSHOT.ground, checkedAt: sentAt },
  });
  render(view());
  expect(await screen.findByText(en["debug.ground.noAirport"])).toBeInTheDocument();
  expect(screen.getByText(/^No airport found \d+ s ago$/)).toBeInTheDocument();
});

it("shows the connection, adapter, profile and last report in the status strip", async () => {
  render(view());
  const strip = await screen.findByRole("status");
  expect(await within(strip).findByText(en["debug.connected"])).toBeInTheDocument();
  expect(within(strip).getByText("SimConnect")).toBeInTheDocument();
  expect(within(strip).getByText("Profile: fenix-a32x")).toBeInTheDocument();
  expect(within(strip).getByText(/Last report \d+ s ago · preview/)).toBeInTheDocument();
});

it("shows the report exactly as built, marked as a preview, and copies it", async () => {
  const user = userEvent.setup();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  render(view());
  await user.click(await screen.findByRole("tab", { name: en["debug.tab.payload"] }));

  expect(await screen.findByText(/"name": "7"/)).toBeInTheDocument();
  expect(screen.getByText(en["debug.payload.previewNote"])).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: en["debug.copyJson"] }));
  expect(writeText).toHaveBeenCalledWith(atGate7.report.json);
});

it("copy does nothing without a clipboard", async () => {
  const user = userEvent.setup();
  Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
  render(view());
  await user.click(await screen.findByRole("tab", { name: en["debug.tab.payload"] }));
  await user.click(await screen.findByRole("button", { name: en["debug.copyJson"] }));
  expect(screen.getByRole("button", { name: en["debug.copyJson"] })).toBeInTheDocument();
});
