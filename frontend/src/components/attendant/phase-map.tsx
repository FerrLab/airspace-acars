import { useTranslation } from "react-i18next";
import { Plane } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  DIVERSION_POINTS, MAP_MARKERS, MAP_VIEWBOX, PROFILE_POINTS,
  aircraftMarker, clipKey, markerState, stepLabelKey, stepsOf,
  type AttendantSnapshot, type MarkerState,
} from "@/lib/attendant";

const MARKER_CLASS: Record<MarkerState, string> = {
  fired: "fill-primary stroke-primary",
  playing: "fill-amber-400 stroke-amber-300",
  pending: "fill-background stroke-muted-foreground",
  silent: "fill-background stroke-muted-foreground opacity-35",
  skipped: "fill-background stroke-muted-foreground opacity-50",
};

/**
 * The flight profile with a marker per phase, drawn as the Airspace speech
 * pack editor draws it, so a pilot sees where each announcement belongs and
 * which have played.
 */
export function PhaseMap({ snapshot }: { snapshot: AttendantSnapshot }) {
  const { t } = useTranslation();
  const steps = stepsOf(snapshot);
  const byKey = new Map(steps.map((s) => [s.key, s]));
  const playing = clipKey(snapshot.nowPlaying);
  const plane = aircraftMarker(snapshot);
  const seatbeltOff = byKey.get("seatbelt_off");
  const seatbeltOn = byKey.get("seatbelt_on");

  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <h3 className="mb-2 text-xs font-semibold">{t("attendant.map")}</h3>
      <svg viewBox={MAP_VIEWBOX} className="h-auto w-full" role="group" aria-label={t("attendant.map")}>
        <defs>
          <filter id="attendant-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <line x1="0" y1="140" x2="1000" y2="140" className="stroke-border" strokeWidth="1" />
        <polyline points={PROFILE_POINTS} fill="none" className="stroke-muted-foreground/60" strokeWidth="2" />
        <polyline points={DIVERSION_POINTS} fill="none" className="stroke-muted-foreground/60" strokeWidth="2" strokeDasharray="6 5" />

        <g role="img" aria-label={`${t("attendant.seatbeltSign")}: ${t("attendant.signOff")} ${snapshot.seatbelts.off}, ${t("attendant.signOn")} ${snapshot.seatbelts.on}`}>
          <rect x="430" y="62" width="180" height="26" rx="13" className="fill-muted stroke-border" />
          <text x="444" y="79" className="fill-muted-foreground text-[10px]">{t("attendant.seatbeltSign")}</text>
          <SignChip x={522} label={`${t("attendant.signOff")} ${snapshot.seatbelts.off}`}
            state={markerState(seatbeltOff, playing)} />
          <SignChip x={567} label={`${t("attendant.signOn")} ${snapshot.seatbelts.on}`}
            state={markerState(seatbeltOn, playing)} />
        </g>

        {MAP_MARKERS.map((m) => {
          const step = byKey.get(m.key);
          if (!step && m.key.startsWith("meal_service")) return null;
          const state = markerState(step, playing);
          const label = t(stepLabelKey(m.key));
          return (
            <g key={m.key} role="img" aria-label={`${label}: ${t(`attendant.status.${state}`)}`}>
              <circle
                cx={m.x} cy={m.y} r={state === "playing" ? 8 : 5.5} strokeWidth="2"
                className={cn(MARKER_CLASS[state], state === "skipped" && "[stroke-dasharray:2_2]")}
                filter={state === "playing" ? "url(#attendant-glow)" : undefined}
              />
              <text x={m.x} y={m.y + (m.y > 120 ? 24 : -14)} textAnchor="middle"
                className={cn("text-[11px]", state === "silent" ? "fill-muted-foreground/50" : "fill-muted-foreground")}>
                {label}
              </text>
            </g>
          );
        })}

        {/* The aircraft sits on the line over its phase's marker: airborne
            labels sit above the line and ground ones below, so off the line
            there is no side it clears for every phase. The disc masks the
            line and marker, which a stroke-only icon would show through. */}
        {plane && (
          <g role="img" aria-label={t("attendant.aircraft")}>
            <circle cx={plane.x} cy={plane.y} r="10" className="fill-card" />
            <Plane x={plane.x - 9} y={plane.y - 9} width={18} height={18} className="text-primary" />
          </g>
        )}
      </svg>
    </section>
  );
}

function SignChip({ x, label, state }: { x: number; label: string; state: MarkerState }) {
  return (
    <g>
      <rect x={x} y="66" width="40" height="18" rx="9" strokeWidth="1.5" className={MARKER_CLASS[state]} />
      <text x={x + 20} y="79" textAnchor="middle"
        className={cn("text-[9px]", state === "fired" ? "fill-primary-foreground" : "fill-foreground")}>
        {label}
      </text>
    </g>
  );
}
