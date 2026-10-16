import { useRef } from "react";
import { render, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { Events } from "@wailsio/runtime";
import * as sounds from "@/lib/notification-sounds";
import { useAutoFlightNotices } from "./use-auto-flight-notices";
import en from "@/locales/en.json";

const i18n = createInstance();
await i18n.init({
  lng: "en",
  resources: { en: { translation: en } },
  keySeparator: false,
  interpolation: { escapeValue: false },
});

type Handler = (event: { data?: unknown }) => void;
let handlers: Record<string, Handler>;
let unsubscribed: string[];
const notify = vi.fn();

function Harness() {
  const volumeRef = useRef(40);
  useAutoFlightNotices(notify, volumeRef);
  return null;
}

function mount() {
  return render(
    <I18nextProvider i18n={i18n}>
      <Harness />
    </I18nextProvider>
  );
}

beforeEach(() => {
  handlers = {};
  unsubscribed = [];
  vi.spyOn(Events, "On").mockImplementation(((name: string, cb: Handler) => {
    handlers[name] = cb;
    return () => unsubscribed.push(name);
  }) as any);
  vi.spyOn(sounds, "playAutoStartDing").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  notify.mockReset();
});

it("announces an automatic start with the chime", () => {
  mount();
  handlers["auto-flight-start"]({ data: "AZU4512" });

  expect(notify).toHaveBeenCalledWith("Flight auto-started: AZU4512");
  expect(sounds.playAutoStartDing).toHaveBeenCalledWith(40);
});

it("announces an automatic finish at the arrival airport", () => {
  mount();
  handlers["auto-flight-finish"]({ data: "SBSP" });

  expect(notify).toHaveBeenCalledWith("Parked at SBSP: finishing the flight automatically");
});

// The pilot is told why nothing happened, and that the button still works.
it("explains a start or finish skipped away from the booked airport", () => {
  mount();
  handlers["auto-flight-skipped"]({ data: { action: "start", airport: "SBGR" } });
  handlers["auto-flight-skipped"]({ data: { action: "finish", airport: "SBRJ" } });

  expect(notify.mock.calls).toEqual([
    [en["acars.autoStartSkipped"].replace("{{airport}}", "SBGR")],
    [en["acars.autoFinishSkipped"].replace("{{airport}}", "SBRJ")],
  ]);
});

it("ignores a skip it cannot read", () => {
  mount();
  handlers["auto-flight-skipped"]({ data: "garbage" });
  handlers["auto-flight-skipped"]({});

  expect(notify).not.toHaveBeenCalled();
});

it("unsubscribes on unmount", () => {
  const { unmount } = mount();
  unmount();

  expect(unsubscribed.sort()).toEqual(["auto-flight-finish", "auto-flight-skipped", "auto-flight-start"]);
});
