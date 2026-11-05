import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { AttendantService } from "../../bindings/airspace-acars";
import { EMPTY_ATTENDANT, type AttendantSnapshot } from "@/lib/attendant";
import en from "@/locales/en.json";
import { AttendantTab } from "./attendant-tab";

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

const firedAt = "2026-11-03T18:05:00Z";
const inCruise: AttendantSnapshot = {
  ...EMPTY_ATTENDANT,
  mode: "client",
  current: "cruise",
  steps: [
    { key: "gate_departure", phase: "gate_departure", status: "fired", lines: 2, firedAt, count: 1, reason: { code: "gsx_boarding_started" } },
    { key: "pushback", phase: "pushback", status: "skipped", lines: 0, firedAt: "0001-01-01T00:00:00Z", count: 0, reason: { code: "" } },
    { key: "cruise", phase: "cruise", status: "fired", lines: 1, firedAt, count: 1, reason: { code: "cruise", altitude: 36000, seconds: 120 } },
    { key: "meal_service:1", phase: "meal_service", service: 1, status: "pending", lines: 1, firedAt: "0001-01-01T00:00:00Z", count: 0, reason: { code: "" } },
    { key: "seatbelt_on", phase: "seatbelt_on", status: "fired", lines: 1, firedAt, count: 2, reason: { code: "seatbelt_on", altitude: 23400 } },
  ],
  nowPlaying: {
    id: "clip-7", lineId: 4, phase: "seatbelt_on", text: "The captain has switched on the seatbelt sign.",
    language: "en", voice: "Lily", music: false, reason: { code: "seatbelt_on", altitude: 23400 }, segments: [],
  },
  history: [{
    at: firedAt, phase: "gate_departure", text: "Welcome aboard", language: "en", voice: "Lily", music: false,
    outcome: "played", reason: { code: "gsx_boarding_started" },
  }],
  gsx: { ...EMPTY_ATTENDANT.gsx, connected: true, boarding: { state: 6, completed: true, progress: "180/180" } },
  seatbelt: { seatbeltSign: true, seatbeltKnown: true },
  seatbelts: { on: 2, off: 1 },
};

function view() {
  return render(<I18nextProvider i18n={i18n}><AttendantTab volume={70} onVolumeChange={() => {}} /></I18nextProvider>);
}

beforeEach(() => {
  vi.spyOn(AttendantService, "GetSnapshot").mockResolvedValue(inCruise);
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("shows what is playing, in which language and voice, and why", async () => {
  view();
  expect(await screen.findByText("The captain has switched on the seatbelt sign.")).toBeInTheDocument();
  expect(screen.getByText("EN · Lily")).toBeInTheDocument();
  expect(screen.getAllByText("Seatbelt sign switched on at 23,400 ft").length).toBeGreaterThan(0);
  expect(screen.getByRole("status")).toHaveTextContent(en["attendant.mode.client"]);
});

it("draws the phase map with each marker's state and the aircraft", async () => {
  view();
  const map = await screen.findByRole("group", { name: en["attendant.map"] });
  expect(within(map).getByRole("img", { name: "Boarding: Played" })).toBeInTheDocument();
  expect(within(map).getByRole("img", { name: "Pushback: Silent" })).toBeInTheDocument();
  expect(within(map).getByRole("img", { name: "Meal 1: Pending" })).toBeInTheDocument();
  expect(within(map).getByRole("img", { name: "Seatbelt sign: Off 1, On 2" })).toBeInTheDocument();
  expect(within(map).getByRole("img", { name: en["attendant.aircraft"] })).toBeInTheDocument();
});

it("lists the phases with their lines, time and reason, and the history", async () => {
  view();
  const cruise = await screen.findByRole("listitem", { name: "Cruise" });
  expect(cruise).toHaveTextContent("Lines: 1");
  expect(cruise).toHaveTextContent("Level at 36,000 ft for 120 s");
  expect(screen.getByText("Welcome aboard")).toBeInTheDocument();
  expect(screen.getByText("Completed")).toBeInTheDocument();
  expect(screen.getByText("· 180/180")).toBeInTheDocument();
});

it("sends the pilot's controls to the attendant", async () => {
  const skip = vi.spyOn(AttendantService, "Skip");
  const stop = vi.spyOn(AttendantService, "StopAll");
  const play = vi.spyOn(AttendantService, "PlayPhase");
  const enable = vi.spyOn(AttendantService, "SetEnabled");
  view();
  await screen.findByText("The captain has switched on the seatbelt sign.");

  fireEvent.click(screen.getByRole("button", { name: en["attendant.skip"] }));
  fireEvent.click(screen.getByRole("button", { name: en["attendant.stop"] }));
  fireEvent.click(screen.getByRole("button", { name: "Play now: Meal 1" }));
  fireEvent.click(screen.getByRole("switch", { name: en["attendant.enabled"] }));
  expect(skip).toHaveBeenCalled();
  expect(stop).toHaveBeenCalled();
  expect(play).toHaveBeenCalledWith("meal_service", 1);
  expect(enable).toHaveBeenCalledWith(false);
});

it("says why nothing plays against an old server", async () => {
  vi.mocked(AttendantService.GetSnapshot).mockResolvedValue({ ...EMPTY_ATTENDANT, mode: "legacy" });
  view();
  await act(async () => { await Promise.resolve(); });
  expect(screen.getByRole("status")).toHaveTextContent(en["attendant.mode.legacy"]);
  expect(screen.queryByRole("button", { name: /Play now/ })).not.toBeInTheDocument();
});

it("shows the server's progress, not an error, while the pack is being prepared", async () => {
  vi.mocked(AttendantService.GetSnapshot).mockResolvedValue({
    ...EMPTY_ATTENDANT, mode: "loading", preparing: { linesReady: 12, linesTotal: 28 },
  });
  view();
  await act(async () => { await Promise.resolve(); });
  const status = screen.getByRole("status");
  expect(status).toHaveTextContent("Preparing announcements (12/28)");
  expect(status).not.toHaveTextContent(en["attendant.mode.loading"]);
});
