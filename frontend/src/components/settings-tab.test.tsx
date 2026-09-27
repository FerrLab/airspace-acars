import { act, fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { SettingsService, DiscordService } from "../../bindings/airspace-acars";
import { SettingsTab } from "./settings-tab";
import * as audioManager from "@/lib/audio-manager";
import en from "@/locales/en.json";

const i18n = createInstance();
await i18n.init({
  lng: "en",
  resources: { en: { translation: en } },
  keySeparator: false,
  interpolation: { escapeValue: false },
});

function view() {
  return (
    <I18nextProvider i18n={i18n}>
      <SettingsTab />
    </I18nextProvider>
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.spyOn(SettingsService, "GetSettings").mockResolvedValue({
    theme: "dark",
    simType: "auto",
    xplaneHost: "127.0.0.1",
    xplanePort: 49000,
    apiBaseURL: "https://airspace.ferrlab.com",
    apiKey: "",
    localMode: false,
    chatSound: "default",
    discordPresence: true,
    language: "en",
    autoStartFlight: true,
    confirmCloseApp: false,
    confirmCancelFlight: false,
    confirmFinishFlight: false,
    aircraftProfile: "auto",
    audioOutputDevice: "default",
  } as any);

  vi.spyOn(SettingsService, "UpdateSettings").mockResolvedValue();
  vi.spyOn(DiscordService, "SetEnabled").mockResolvedValue();

  const mockDevices: audioManager.AudioOutputDeviceInfo[] = [
    { deviceId: "default", label: "System Default", isDefault: true },
    { deviceId: "headphones-1", label: "Headphones (HyperX Cloud)", isDefault: false },
    { deviceId: "virtual-cable", label: "CABLE Input (VB-Audio)", isDefault: false },
  ];
  vi.spyOn(audioManager, "getAudioOutputDevices").mockResolvedValue(mockDevices);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
});

it("renders audio device selector, master volume, and test button", async () => {
  render(view());

  expect(await screen.findByText("Audio & Notifications")).toBeInTheDocument();
  expect(screen.getByText("Audio Output Device")).toBeInTheDocument();
  expect(screen.getByText("Master Volume")).toBeInTheDocument();
  expect(screen.getByText("Chat notification sound")).toBeInTheDocument();
  expect(screen.getByText("Test Audio")).toBeInTheDocument();
});

it("triggers test sound when clicking Test Audio button", async () => {
  const playSpy = vi.spyOn(audioManager, "playDeviceTestSound").mockResolvedValue();
  render(view());

  const testBtn = await screen.findByText("Test Audio");
  await act(async () => {
    fireEvent.click(testBtn);
  });

  expect(playSpy).toHaveBeenCalled();
});

it("adjusts master volume slider and dispatches change event", async () => {
  render(view());
  await screen.findByText("Audio & Notifications");

  const slider = screen.getByRole("slider");
  await act(async () => {
    fireEvent.change(slider, { target: { value: "75" } });
  });

  expect(screen.getByText("75%")).toBeInTheDocument();
  expect(localStorage.getItem("acars_volume")).toBe("75");
});
