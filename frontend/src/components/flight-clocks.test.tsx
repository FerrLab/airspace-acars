import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { clockReading, FlightClocks } from "./flight-clocks";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: "en-GB" } }) }));
afterEach(() => { cleanup(); vi.useRealTimers(); });

it("keeps Zulu at UTC across midnight while the local calendar can be the previous day", () => {
  const now = new Date("2026-09-27T00:05:09Z");
  expect(clockReading(now, "en-GB", "UTC")).toEqual({ time: "00:05:09", date: "27 Sept 2026" });
  expect(clockReading(now, "en-GB", "America/Sao_Paulo")).toEqual({ time: "21:05:09", date: "26 Sept 2026" });
});

it("respects daylight saving in the selected timezone", () => {
  expect(clockReading(new Date("2026-01-15T12:00:00Z"), "en-GB", "America/New_York").time).toBe("07:00:00");
  expect(clockReading(new Date("2026-07-15T12:00:00Z"), "en-GB", "America/New_York").time).toBe("08:00:00");
});

it("uses the OS clock, updates every second, resyncs on focus and clears its timer", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-26T23:59:59Z"));
  const { unmount } = render(<FlightClocks />);
  expect(screen.getByTestId("clock-zulu")).toHaveTextContent("23:59:59");
  expect(screen.getByTestId("clock-local")).toHaveTextContent(new Date().toLocaleTimeString("en-GB", { hourCycle: "h23" }));
  act(() => vi.advanceTimersByTime(1000));
  expect(screen.getByTestId("clock-zulu")).toHaveTextContent("00:00:00");
  vi.setSystemTime(new Date("2026-09-27T10:20:30Z"));
  fireEvent.focus(window);
  expect(screen.getByTestId("clock-zulu")).toHaveTextContent("10:20:30");
  unmount();
  expect(vi.getTimerCount()).toBe(0);
});
