import { describe, expect, it } from "vitest";
import {
  EMPTY_ATTENDANT, MAP_MARKERS, aircraftMarker, clipKey, clockTime, formatSigned, gsxStateKey,
  languageVoice, legacyPollActive, markerState, reasonText, stepLabelKey, stepsOf,
  type AttendantStep,
} from "./attendant";

const step = (over: Partial<AttendantStep>): AttendantStep => ({
  key: "cruise", phase: "cruise", status: "pending", lines: 2, firedAt: "0001-01-01T00:00:00Z",
  count: 0, reason: { code: "" }, ...over,
});

describe("markerState", () => {
  it("draws playing over fired, and a phase without lines as silent", () => {
    expect(markerState(step({ status: "fired" }), "cruise")).toBe("playing");
    expect(markerState(step({ status: "fired" }), "")).toBe("fired");
    expect(markerState(step({ status: "skipped" }), "")).toBe("skipped");
    expect(markerState(step({ lines: 0, status: "fired" }), "")).toBe("silent");
    expect(markerState(step({}), "")).toBe("pending");
    expect(markerState(undefined, "")).toBe("pending");
  });
});

describe("the phase map", () => {
  it("places every marker the editor draws, meal services by number", () => {
    const keys = MAP_MARKERS.map((m) => m.key);
    expect(keys).toContain("meal_service:1");
    expect(keys).toContain("alternate_declared");
    expect(MAP_MARKERS.find((m) => m.key === "cruise")).toEqual({ key: "cruise", x: 440, y: 40 });
  });

  it("shows every phase pending before a flight has a pack", () => {
    const steps = stepsOf(EMPTY_ATTENDANT);
    expect(steps.map((s) => s.key)).toContain("seatbelt_on");
    expect(steps.every((s) => s.status === "pending")).toBe(true);
    expect(steps.find((s) => s.key === "meal_service:2")?.service).toBe(2);
  });

  it("puts the aircraft at the last phase, at the gate once a flight starts, and nowhere before", () => {
    expect(aircraftMarker(EMPTY_ATTENDANT)).toBeNull();
    expect(aircraftMarker({ ...EMPTY_ATTENDANT, mode: "client" })?.key).toBe("gate_departure");
    expect(aircraftMarker({ ...EMPTY_ATTENDANT, mode: "client", current: "descend" })).toEqual({ key: "descend", x: 680, y: 48 });
  });

  it("keys a meal service clip by its service", () => {
    expect(clipKey({ phase: "meal_service", service: 2 })).toBe("meal_service:2");
    expect(clipKey({ phase: "taxi" })).toBe("taxi");
    expect(clipKey(null)).toBe("");
    expect(stepLabelKey("meal_service:2")).toBe("attendant.phase.meal_service_2");
  });
});

describe("words", () => {
  const t = (key: string, o?: Record<string, unknown>) => `${key}${o ? " " + JSON.stringify(o) : ""}`;

  it("fills a reason with formatted numbers", () => {
    const text = reasonText(t, { code: "touchdown", vs: -142, g: 1.08, landing: "smooth" });
    expect(text).toContain("attendant.reason.touchdown");
    expect(text).toContain('"vs":"−142"');
    expect(text).toContain('"g":"1.08"');
    expect(text).toContain('"landing":"attendant.landing.smooth"');
    expect(reasonText(t, { code: "seatbelt_on", altitude: 23400 })).toContain('"altitude":"23,400"');
    expect(reasonText(t, { code: "" })).toBe("");
    expect(reasonText(t, null)).toBe("");
  });

  it("formats signs and voices", () => {
    expect(formatSigned(-1500)).toBe("−1,500");
    expect(formatSigned(300)).toBe("+300");
    expect(formatSigned(0)).toBe("0");
    expect(languageVoice({ language: "en", voice: "Lily" })).toBe("EN · Lily");
    expect(languageVoice({ language: "", voice: "" })).toBe("");
  });

  it("reads Go times and GSX states", () => {
    expect(clockTime("0001-01-01T00:00:00Z")).toBe("");
    expect(clockTime("garbage")).toBe("");
    expect(clockTime(new Date(2026, 10, 3, 9, 5).toISOString())).toBe("09:05");
    expect(gsxStateKey(5)).toBe("attendant.gsx.state.active");
    expect(gsxStateKey(42)).toBe("attendant.gsx.state.unknown");
  });
});

describe("legacyPollActive", () => {
  it("polls GET /sound only against an old server, and only when on", () => {
    expect(legacyPollActive({ mode: "legacy", enabled: true })).toBe(true);
    expect(legacyPollActive({ mode: "legacy", enabled: false })).toBe(false);
    expect(legacyPollActive({ mode: "client", enabled: true })).toBe(false);
    expect(legacyPollActive({ mode: "loading", enabled: true })).toBe(false);
  });
});
