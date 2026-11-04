import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import en from "@/locales/en.json";
import type { FlightData } from "@/hooks/use-flight-data";
import { DebugTelemetry } from "./debug-telemetry";

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

const engine = { exists: true, running: false, n1: 0, n2: 0, throttlePos: 0, mixturePos: 0, propPos: 0 };
const base: FlightData = {
  position: { latitude: 0, longitude: 0, altitude: 0, altitudeAGL: 0 },
  attitude: { pitch: 0, roll: 0, headingTrue: 0, headingMag: 0, vs: 0, ias: 0, tas: 0, gs: 0, gForce: 1 },
  engines: [engine, engine, engine, engine],
  sensors: { onGround: true, stallWarning: false, overspeedWarning: false, simulationRate: 1, paused: false, slew: false, crashed: false },
  radios: { com1: 0, com2: 0, nav1: 0, nav2: 0, nav1OBS: 0, nav2OBS: 0, xpdrCode: 0, xpdrState: "" },
  autopilot: { master: false, heading: 0, altitude: 0, vs: 0, speed: 0, approachHold: false, navLock: false },
  altimeterInHg: 29.92,
  lights: { beacon: false, strobe: false, landing: false },
  controls: { elevator: 0, aileron: 0, rudder: 0, flaps: 0, spoilers: 0, gearDown: true },
  simTime: { zuluTime: 0, zuluDay: 1, zuluMonth: 1, zuluYear: 2026, localTime: 0 },
  apu: { switchOn: false, rpmPercent: 0, genSwitch: false, genActive: false },
  doors: [],
  aircraftName: "",
  weight: { totalWeight: 0, fuelWeight: 0 },
} as unknown as FlightData;

afterEach(() => cleanup());

function view(d: FlightData) {
  return render(<I18nextProvider i18n={i18n}><DebugTelemetry flightData={d} /></I18nextProvider>);
}

// The flight attendant announces the seatbelt sign only when an aircraft
// actually reports it, so the debug screen shows "not read" apart from "off".
it("shows the seatbelt sign, and says when nothing reads it", () => {
  view(base);
  expect(screen.getByRole("row", { name: /Seatbelt sign\s*not read/ })).toBeInTheDocument();
  cleanup();
  view({ ...base, cabin: { seatbeltSign: true, seatbeltKnown: true } });
  expect(screen.getByRole("row", { name: /Seatbelt sign\s*ON/ })).toBeInTheDocument();
});
