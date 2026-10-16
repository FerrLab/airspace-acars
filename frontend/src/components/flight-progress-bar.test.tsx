import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FlightProgressBar } from "./flight-progress-bar";

describe("FlightProgressBar", () => {
  const sbgr = { lat: -23.4356, lon: -46.4731 };
  const sawh = { lat: -54.8433, lon: -68.2956 };

  it("renders with 0% progress and 0:00 elapsed in idle flight state", () => {
    render(
      <FlightProgressBar
        flightState="idle"
        departureCoords={sbgr}
        arrivalCoords={sawh}
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
        departureCoords={sbgr}
        arrivalCoords={sawh}
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
        departureCoords={sbgr}
        arrivalCoords={sawh}
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

  it("renders indeterminate state without fake progress when destination coordinates are unknown", () => {
    render(
      <FlightProgressBar
        flightState="active"
        departureCoords={sbgr}
        arrivalCoords={null}
        currentLat={-39.1}
        currentLon={-57.4}
        onGround={false}
        groundSpeed={440}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    expect(progressBar).toBeInTheDocument();
    expect(progressBar).not.toHaveAttribute("aria-valuenow");
    expect(screen.getByText("—:—")).toBeInTheDocument();
  });

  it("renders elapsed time from flightStartTime in active flight state", () => {
    const twentyMinAgo = Date.now() - 20 * 60 * 1000;
    render(
      <FlightProgressBar
        flightState="active"
        departureCoords={sbgr}
        arrivalCoords={sawh}
        flightStartTime={twentyMinAgo}
      />
    );

    const progressBar = screen.getByRole("progressbar");
    expect(progressBar).toBeInTheDocument();
    expect(screen.getAllByText("0:20").length).toBeGreaterThanOrEqual(1);
  });
});
