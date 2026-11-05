import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import L from "leaflet";
import { FlightsMap } from "./flights-map";
import { ThemeProvider } from "../context/theme-context";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
    i18n: { language: "en" },
  }),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// Leaflet's panes and controls carry z-index 400–1000. The map must keep
// them inside its own stacking context, or they paint over app-level
// overlays such as the close-confirmation modal (z-50) and leave the map
// bright on top of the dimmed backdrop.
describe("FlightsMap stacking", () => {
  it("keeps Leaflet's z-indexes inside its own stacking context", () => {
    render(
      <ThemeProvider>
        <FlightsMap flights={[]} selectedFlightId={null} onSelectFlight={() => {}} />
      </ThemeProvider>
    );

    expect(
      screen.getByTestId("flights-map-root").className.split(/\s+/),
      "the map root does not isolate its stacking context, so modals open underneath the map"
    ).toContain("isolate");
  });
});

// Contract: leaving the map must not leave a Leaflet timer behind that fires
// on the removed map (Sentry AIRSPACE-ACARS-M, "reading '_leaflet_pos'").
describe("FlightsMap teardown", () => {
  it("creates the map without zoom animation, whose timer outlives map.remove()", () => {
    const mapSpy = vi.spyOn(L, "map");
    render(
      <ThemeProvider>
        <FlightsMap flights={[]} selectedFlightId={null} onSelectFlight={() => {}} />
      </ThemeProvider>
    );

    expect(
      mapSpy.mock.calls[0]?.[1]?.zoomAnimation,
      "zoom animation is on, so closing the tab mid-zoom throws from Leaflet's timer"
    ).toBe(false);
  });
});
