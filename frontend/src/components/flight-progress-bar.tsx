import { useMemo, useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  computeFlightProgress,
  formatFlightDuration,
  type Coordinates,
} from "@/lib/flight-progress";

export interface FlightProgressBarProps {
  flightState: "idle" | "active" | "finishing";
  departureCoords?: Coordinates | null;
  arrivalCoords?: Coordinates | null;
  currentLat?: number | null;
  currentLon?: number | null;
  onGround?: boolean;
  groundSpeed?: number;
  flightStartTime?: number | Date | string | null;
  cruiseSpeed?: number;
  className?: string;
}

export function FlightProgressBar({
  flightState,
  departureCoords,
  arrivalCoords,
  currentLat,
  currentLon,
  onGround = false,
  groundSpeed = 0,
  flightStartTime,
  cruiseSpeed,
  className,
}: FlightProgressBarProps) {
  const { t } = useTranslation();

  // Remember initial aircraft position when tracking starts
  const [initialPos, setInitialPos] = useState<Coordinates | null>(null);

  // Live timer ticker and fallback internal start time
  const [internalStartTime, setInternalStartTime] = useState<number | null>(null);
  const [currentTime, setCurrentTime] = useState<number>(() => Date.now());

  useEffect(() => {
    if (flightState === "active" && !initialPos && currentLat && currentLon) {
      setInitialPos({ lat: currentLat, lon: currentLon });
    } else if (flightState === "idle") {
      setInitialPos(null);
    }
  }, [flightState, currentLat, currentLon, initialPos]);

  useEffect(() => {
    if (flightState === "active") {
      if (!internalStartTime && !flightStartTime) {
        setInternalStartTime(Date.now());
      }
      // Tick every 1 second so elapsed flight duration updates live in real time
      const timer = window.setInterval(() => {
        setCurrentTime(Date.now());
      }, 1000);
      return () => window.clearInterval(timer);
    } else if (flightState === "idle") {
      setInternalStartTime(null);
    }
  }, [flightState, internalStartTime, flightStartTime]);

  const effectiveStartTime = flightStartTime ?? internalStartTime;

  const {
    progressPercent,
    totalDistanceNm,
    statusPhase,
    elapsedMinutes,
    remainingMinutes,
    durationMinutes,
  } = useMemo(
    () =>
      computeFlightProgress({
        flightState,
        departureCoords,
        arrivalCoords,
        currentLat,
        currentLon,
        onGround,
        groundSpeed,
        initialRecordedPosition: initialPos,
        flightStartTime: effectiveStartTime,
        currentTime,
        cruiseSpeed,
      }),
    [
      flightState,
      departureCoords,
      arrivalCoords,
      currentLat,
      currentLon,
      onGround,
      groundSpeed,
      initialPos,
      effectiveStartTime,
      currentTime,
      cruiseSpeed,
    ]
  );

  const isIndeterminate = progressPercent === null;
  const roundedProgress = isIndeterminate ? undefined : Math.round(progressPercent);
  const clampedProgress = isIndeterminate ? 0 : Math.min(100, Math.max(0, progressPercent));

  const elapsedText = formatFlightDuration(elapsedMinutes);
  const remainingText = remainingMinutes !== null ? formatFlightDuration(remainingMinutes) : "—:—";
  const durationText = durationMinutes !== null ? formatFlightDuration(durationMinutes) : elapsedText;

  return (
    <div
      className={cn("flex flex-col w-full min-w-0 select-none py-1", className)}
      role="progressbar"
      aria-label={t("acars.dashboard.flightProgress")}
      aria-valuenow={roundedProgress}
      aria-valuemin={isIndeterminate ? undefined : 0}
      aria-valuemax={isIndeterminate ? undefined : 100}
    >
      {/* Top Status / Phase Header */}
      <div className="text-center mb-1.5">
        <span
          className={cn(
            "text-xs sm:text-sm font-bold tracking-wide transition-colors",
            flightState === "active"
              ? "text-yellow-400 drop-shadow-[0_0_8px_rgba(250,204,21,0.4)]"
              : flightState === "finishing"
                ? "text-emerald-400"
                : "text-yellow-400/90"
          )}
        >
          {flightState === "active"
            ? statusPhase === "approach"
              ? t("acars.dashboard.approachPhase")
              : statusPhase === "taxi"
                ? t("acars.dashboard.preflightPhase")
                : t("acars.dashboard.enRoute")
            : flightState === "finishing"
              ? t("acars.dashboard.completedPhase")
              : t("acars.dashboard.preflightPhase")}
        </span>
      </div>

      {/* Main Row: Elapsed (left) ➔ Track with leading plane (center) ➔ Remaining (right) */}
      <div className="flex items-center w-full gap-3 sm:gap-4">
        {/* Left: Elapsed */}
        <div className="flex flex-col items-start min-w-[48px] shrink-0 text-left">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t("acars.dashboard.elapsed")}
          </span>
          <span className="font-mono text-xs sm:text-sm font-bold text-foreground tabular-nums">
            {elapsedText}
          </span>
        </div>

        {/* Center: Flight Track */}
        <div className="relative flex-1 h-6 flex items-center min-w-0 mx-1 sm:mx-2">
          {/* Background Muted Track Line */}
          <div
            className="absolute inset-x-0 top-1/2 -translate-y-1/2 h-[3px] bg-muted/60 dark:bg-zinc-700/60 rounded-full"
            aria-hidden="true"
          />

          {/* Active Solid Yellow/Green Line */}
          {!isIndeterminate && (
            <div
              className={cn(
                "absolute left-0 top-1/2 -translate-y-1/2 h-[3px] rounded-l-full transition-all duration-700 ease-out",
                flightState === "finishing"
                  ? "bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.6)]"
                  : "bg-yellow-400 shadow-[0_0_10px_rgba(250,204,21,0.6)]"
              )}
              style={{ width: `${clampedProgress}%` }}
              aria-hidden="true"
            />
          )}

          {/* Airplane: Centered directly ON the progress line - only when progress is determinate */}
          {!isIndeterminate && (
            <div
              className="absolute top-1/2 -translate-x-1/2 -translate-y-1/2 z-10 transition-all duration-700 ease-out pointer-events-none"
              style={{
                left: `${clampedProgress}%`,
              }}
            >
              <div className="relative flex items-center justify-center">
                {/* Contrail jet glow directly behind airplane tail */}
                {flightState === "active" && clampedProgress > 0 && (
                  <span
                    className="absolute -left-2 top-1/2 -translate-y-1/2 h-1 w-2.5 bg-yellow-400/80 blur-[2px] rounded-full"
                    aria-hidden="true"
                  />
                )}

                {/* Symmetrical Top-Down Airplane Icon */}
                <svg
                  viewBox="0 0 24 24"
                  fill="currentColor"
                  className={cn(
                    "h-5 w-5 transition-transform duration-300",
                    flightState === "active"
                      ? "text-yellow-400 drop-shadow-[0_0_10px_rgba(250,204,21,0.85)]"
                      : flightState === "finishing"
                        ? "text-emerald-400 drop-shadow-[0_0_8px_rgba(16,185,129,0.8)]"
                        : "text-yellow-400 drop-shadow-[0_0_6px_rgba(250,204,21,0.6)]"
                  )}
                  aria-hidden="true"
                >
                  <g transform="rotate(90 11.5 12)">
                    <path d="M21 16v-2l-8-5V3.5c0-.83-.67-1.5-1.5-1.5S10 2.67 10 3.5V9l-8 5v2l8-2.5V19l-2 1.5V22l3.5-1 3.5 1v-1.5L13 19v-5.5l8 2.5z" />
                  </g>
                </svg>
              </div>
            </div>
          )}
        </div>

        {/* Right: Remaining */}
        <div className="flex flex-col items-end min-w-[48px] shrink-0 text-right">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t("acars.dashboard.remaining")}
          </span>
          <span className="font-mono text-xs sm:text-sm font-bold text-foreground tabular-nums">
            {remainingText}
          </span>
        </div>
      </div>

      {/* Duration below the track */}
      <div className="text-center text-[11px] font-medium text-muted-foreground mt-1 whitespace-nowrap">
        <span>{t("acars.dashboard.duration")} </span>
        <span className="font-mono font-semibold text-foreground/90 tabular-nums">
          {durationText}
        </span>
        {totalDistanceNm !== null && (
          <span className="text-[10px] text-muted-foreground/70 ml-1.5 hidden sm:inline tabular-nums">
            ({Math.round(totalDistanceNm).toLocaleString()} NM)
          </span>
        )}
      </div>
    </div>
  );
}
