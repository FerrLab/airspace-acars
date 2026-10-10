import { useTranslation } from "react-i18next";
import { CheckCircle2, Wind } from "lucide-react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import {
  getRunwayPerformance,
  getStabilizedGate,
  type RunwayPerformance,
} from "@/lib/flight-detail";

interface RunwayProps {
  flight: FlightLog;
  mode: "landing" | "takeoff";
}

export function FlightDetailRunway({ flight, mode }: RunwayProps) {
  const { t } = useTranslation();
  const perf: RunwayPerformance = getRunwayPerformance(flight, mode);
  const isLanding = mode === "landing";
  const gateItems = getStabilizedGate();

  return (
    <div className="space-y-4">
      {/* Runway Diagram Box */}
      <Card className="border-border/70 bg-card p-5 shadow-xs">
        <div className="flex flex-wrap items-end justify-between gap-3 mb-3">
          <div>
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
              {t("myFlights.runwayDiagram", "Diagrama da Pista")}
            </span>
            <span className="text-base font-bold text-foreground">
              {isLanding ? `Pista ${perf.runwayId} · Toque e Rollout` : `Pista ${perf.runwayId} · Corrida e Decolagem`}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-sky-400" />
              {isLanding ? "Trajetória de toque" : "Trajetória de decolagem"}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-amber-400" />
              {isLanding ? "Ponto de toque" : "Ponto de rotação"}
            </span>
          </div>
        </div>

        {/* SVG Runway Canvas */}
        <div className="relative w-full overflow-hidden rounded-lg bg-zinc-950 p-2 border border-border/50">
          <svg viewBox="0 0 800 200" className="w-full h-auto block select-none">
            {/* Runway tarmac */}
            <rect x="40" y="70" width="720" height="60" fill="#18181b" rx="2" />
            <line x1="40" y1="70" x2="760" y2="70" stroke="#71717a" strokeWidth="1.5" />
            <line x1="40" y1="130" x2="760" y2="130" stroke="#71717a" strokeWidth="1.5" />
            <line x1="40" y1="70" x2="40" y2="130" stroke="#e4e4e7" strokeWidth="2.5" />
            <line x1="760" y1="70" x2="760" y2="130" stroke="#e4e4e7" strokeWidth="2.5" />

            {/* Centerline dashes */}
            <line
              x1="90"
              y1="100"
              x2="710"
              y2="100"
              stroke="#a1a1aa"
              strokeWidth="2"
              strokeDasharray="14 10"
            />

            {/* Threshold stripes */}
            {[76, 82, 88, 94, 106, 112, 118, 124].map((y) => (
              <g key={y}>
                <rect x="46" y={y} width="20" height="3" fill="#f4f4f5" />
                <rect x="734" y={y} width="20" height="3" fill="#f4f4f5" />
              </g>
            ))}

            {/* Aiming points */}
            <rect x="180" y="77" width="28" height="6" fill="#a1a1aa" />
            <rect x="180" y="117" width="28" height="6" fill="#a1a1aa" />
            <rect x="590" y="77" width="28" height="6" fill="#a1a1aa" />
            <rect x="590" y="117" width="28" height="6" fill="#a1a1aa" />

            {/* Aircraft track on runway */}
            {isLanding ? (
              <>
                <path
                  d="M 10 90 Q 120 95, 230 102 L 620 101"
                  fill="none"
                  stroke="#38bdf8"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                />
                <circle cx="230" cy="102" r="8" fill="none" stroke="#f59e0b" strokeWidth="2" />
                <circle cx="230" cy="102" r="3.5" fill="#f59e0b" />
              </>
            ) : (
              <>
                <path
                  d="M 60 100 L 460 98 Q 580 94, 780 75"
                  fill="none"
                  stroke="#38bdf8"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                />
                <circle cx="460" cy="98" r="8" fill="none" stroke="#f59e0b" strokeWidth="2" />
                <circle cx="460" cy="98" r="3.5" fill="#f59e0b" />
              </>
            )}

            {/* Runway numbers */}
            <text
              x="62"
              y="105"
              fill="#e4e4e7"
              fontSize="14"
              fontWeight="bold"
              fontFamily="monospace"
              textAnchor="middle"
              transform="rotate(90 62 105)"
            >
              {perf.runwayId}
            </text>
            <text
              x="738"
              y="105"
              fill="#71717a"
              fontSize="14"
              fontWeight="bold"
              fontFamily="monospace"
              textAnchor="middle"
              transform="rotate(-90 738 105)"
            >
              {perf.recipId}
            </text>
          </svg>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 mt-3 pt-3 border-t border-border/50 text-xs text-muted-foreground">
          <span>{isLanding ? "Ponto de toque a 575 m da cabeceira" : "Rotação a 2.245 m da cabeceira"}</span>
          <span className="font-mono">Largura 45m · Comprimento 2.795m · Asfalto</span>
        </div>
      </Card>

      {/* Grid: Performance Metrics + Wind Rose (for landing) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Performance Table */}
        <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
          <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
            {isLanding
              ? t("myFlights.landingPerformance", "Desempenho de Pouso")
              : t("myFlights.takeoffPerformance", "Desempenho de Decolagem")}
          </span>
          <div className="divide-y divide-border/40 text-xs">
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">{isLanding ? "Tasa de Toque (FPM)" : "Tasa de Subida Inicial"}</span>
              <span className="font-mono font-semibold text-foreground">{perf.rateFpm} ft/min</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">{isLanding ? "G-Force ao Toque" : "G-Force na Rotação"}</span>
              <span className="font-mono font-semibold text-emerald-400">{perf.gForce.toFixed(2)} G</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">{isLanding ? "Distância até o Toque" : "Distância de Corrida"}</span>
              <span className="font-mono font-semibold text-foreground">{perf.distanceM} m</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Desvio do Eixo Central</span>
              <span className="font-mono font-semibold text-foreground">{perf.offsetText}</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Ground Speed</span>
              <span className="font-mono font-semibold text-foreground">{perf.groundSpeedKt} kt</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Indicated Airspeed</span>
              <span className="font-mono font-semibold text-foreground">{perf.airSpeedKt} kt</span>
            </div>
            <div className="flex justify-between py-2">
              <span className="text-muted-foreground">Pista Utilizada</span>
              <span className="font-mono font-semibold text-sky-400">
                {perf.runwayUsedM} m ({perf.runwayUsedPct.toFixed(1)}%)
              </span>
            </div>
          </div>
        </Card>

        {/* Wind at Touchdown Card */}
        {isLanding ? (
          <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block flex items-center gap-1.5">
              <Wind className="h-3.5 w-3.5 text-amber-400" />
              {t("myFlights.windTouchdown", "Vento no Toque")}
            </span>
            <div className="flex items-center gap-4 pt-1">
              {/* Compass Rose */}
              <div className="relative w-28 h-28 shrink-0 flex items-center justify-center">
                <svg viewBox="0 0 120 120" className="w-full h-full">
                  <circle cx="60" cy="60" r="50" fill="none" stroke="rgba(255,255,255,0.15)" strokeWidth="1" />
                  <text x="60" y="16" textAnchor="middle" fill="#a1a1aa" fontSize="9" fontFamily="monospace">N</text>
                  <text x="110" y="63" textAnchor="middle" fill="#71717a" fontSize="9" fontFamily="monospace">E</text>
                  <text x="60" y="112" textAnchor="middle" fill="#71717a" fontSize="9" fontFamily="monospace">S</text>
                  <text x="10" y="63" textAnchor="middle" fill="#71717a" fontSize="9" fontFamily="monospace">W</text>
                  {/* Runway orientation strip */}
                  <g transform={`rotate(${perf.heading} 60 60)`}>
                    <rect x="56" y="24" width="8" height="72" rx="2" fill="#52525b" />
                    <line x1="60" y1="28" x2="60" y2="92" stroke="#a1a1aa" strokeDasharray="3 3" />
                  </g>
                  {/* Wind vector arrow */}
                  <g transform={`rotate(${perf.windHeading} 60 60)`}>
                    <line x1="60" y1="16" x2="60" y2="50" stroke="#f59e0b" strokeWidth="2.5" />
                    <polygon points="56,46 60,56 64,46" fill="#f59e0b" />
                  </g>
                </svg>
              </div>

              {/* Wind Breakdown */}
              <div className="flex-1 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-muted-foreground">Vento Reportado</span>
                  <span className="font-mono font-bold text-foreground">{perf.windHeading}° / {perf.windSpeedKt} kt</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-muted-foreground">Vento de Proa (Headwind)</span>
                  <span className="font-mono font-semibold text-emerald-400">+{perf.headwindKt} kt</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-muted-foreground">Vento de Través (Crosswind)</span>
                  <span className="font-mono font-semibold text-amber-400">
                    {perf.crosswindKt} kt → {perf.crosswindSide}
                  </span>
                </div>
                <p className="text-[10px] text-muted-foreground pt-1">
                  Pista {perf.runwayId} (Rumo magnético {perf.heading}°)
                </p>
              </div>
            </div>
          </Card>
        ) : (
          /* Climb Profile for Takeoff */
          <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
              {t("myFlights.climbProfile", "Perfil de Subida")}
            </span>
            <div className="relative h-28 w-full bg-zinc-950 rounded-lg p-2 border border-border/50">
              <svg viewBox="0 0 400 100" preserveAspectRatio="none" className="w-full h-full">
                <path d="M 10 90 L 100 88 Q 180 80, 240 50 L 390 10" fill="none" stroke="#38bdf8" strokeWidth="2" />
                <path d="M 10 90 L 100 88 Q 180 80, 240 50 L 390 10 L 390 95 L 10 95 Z" fill="#38bdf8" opacity="0.1" />
              </svg>
              <span className="absolute bottom-1 right-2 text-[10px] font-mono text-muted-foreground">
                Gradiente 3.8% · FL100 atingido em 6 min
              </span>
            </div>
          </Card>
        )}
      </div>

      {/* 500 ft Stabilized Gate Checklist (for landing) */}
      {isLanding && (
        <Card className="border-border/70 bg-card p-4 shadow-xs space-y-2.5">
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              {t("myFlights.stabilizedGate", "Gate Estabilizado a 500 ft")}
            </span>
            <Badge variant="outline" className="text-xs bg-emerald-500/10 text-emerald-400 border-emerald-500/30 font-medium">
              STABLE · PASS
            </Badge>
          </div>
          <div className="divide-y divide-border/40 text-xs">
            {gateItems.map((g, idx) => (
              <div key={idx} className="flex items-center justify-between py-2">
                <div className="flex items-center gap-2">
                  <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
                  <span className="text-foreground">{g.defaultLabel}</span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="font-mono text-muted-foreground">{g.value}</span>
                  <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-emerald-500/30 text-emerald-400 bg-emerald-500/10 font-mono">
                    PASS
                  </Badge>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
