import { act, fireEvent, render, screen, cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { SettingsService } from "../../bindings/airspace-acars";
import { AutoFlightSettings } from "./auto-flight-settings";
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
      <AutoFlightSettings />
    </I18nextProvider>
  );
}

const saved = { theme: "dark", simType: "auto", autoStartFlight: true, autoFinishFlight: false };

beforeEach(() => {
  vi.spyOn(SettingsService, "GetSettings").mockResolvedValue({ ...saved } as any);
  vi.spyOn(SettingsService, "UpdateSettings").mockResolvedValue();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The switches show what is saved, so a pilot who turned auto-finish off sees
// it off.
it("shows the saved auto-start and auto-finish settings", async () => {
  render(view());

  const finish = screen.getByRole("switch", { name: en["settings.autoFinishFlight"] });
  await waitFor(() => expect(finish).toHaveAttribute("aria-checked", "false"));
  expect(screen.getByRole("switch", { name: en["settings.autoStartFlight"] })).toHaveAttribute(
    "aria-checked",
    "true"
  );
});

// A settings file written before auto-finish existed has no such key; it
// reads as on, as the backend's default does.
it("reads a missing auto-finish setting as on", async () => {
  vi.spyOn(SettingsService, "GetSettings").mockResolvedValue({ theme: "dark", autoStartFlight: true } as any);
  render(view());

  const finish = screen.getByRole("switch", { name: en["settings.autoFinishFlight"] });
  await waitFor(() => expect(SettingsService.GetSettings).toHaveBeenCalled());
  expect(finish).toHaveAttribute("aria-checked", "true");
});

// Turning a switch saves that one setting and leaves the rest as they are.
it("saves auto-finish without touching the other settings", async () => {
  render(view());
  const finish = screen.getByRole("switch", { name: en["settings.autoFinishFlight"] });
  await waitFor(() => expect(finish).toHaveAttribute("aria-checked", "false"));

  await act(async () => {
    fireEvent.click(finish);
  });

  await waitFor(() => expect(SettingsService.UpdateSettings).toHaveBeenCalledTimes(1));
  expect(SettingsService.UpdateSettings).toHaveBeenCalledWith({ ...saved, autoFinishFlight: true });
  expect(finish).toHaveAttribute("aria-checked", "true");
});
