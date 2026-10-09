import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FlightProgressBar } from "./flight-progress-bar";

describe("FlightProgressBar", () => {
  const sbgr = { icao: "SBGR", city: "São Paulo", latitude: -23.4356, longitude: -46.4731 };
  const sawh = { icao: "SAWH", city: "Ushuaia", latitude: -54.8433, longitude: -68.2956 };

  it("renders with 0% progress and 0:00 elapsed in idle flight state", () => {
    render(
      <FlightProgressBar
        flightState="idle"
        departureAirport={sbgr}
        arrivalAirport={sawh}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    expect(progressBar).toBeInTheDocument();
    expect(progressBar).toHaveAttribute("aria-valuenow", "0");
    expect(screen.getByText("0:00")).toBeInTheDocument();
  });

  it("renders with 100% progress when flight is finishing", () => {
    render(
      <FlightProgressBar
        flightState="finishing"
        departureAirport={sbgr}
        arrivalAirport={sawh}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    expect(progressBar).toBeInTheDocument();
    expect(progressBar).toHaveAttribute("aria-valuenow", "100");
  });

  it("renders dynamic progress during active flight", () => {
    render(
      <FlightProgressBar
        flightState="active"
        departureAirport={sbgr}
        arrivalAirport={sawh}
        currentLat={-39.1}
        currentLon={-57.4}
        onGround={false}
        groundSpeed={440}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    const val = Number(progressBar.getAttribute("aria-valuenow"));
    expect(val).toBeGreaterThanOrEqual(45);
    expect(val).toBeLessThanOrEqual(55);
  });

  it("renders elapsed time from flightStartTime in active flight state", () => {
    const twentyMinAgo = Date.now() - 20 * 60 * 1000;
    render(
      <FlightProgressBar
        flightState="active"
        departureAirport={sbgr}
        arrivalAirport={sawh}
        flightStartTime={twentyMinAgo}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    expect(progressBar).toBeInTheDocument();
    expect(screen.getAllByText("0:20").length).toBeGreaterThanOrEqual(1);
  });
});
