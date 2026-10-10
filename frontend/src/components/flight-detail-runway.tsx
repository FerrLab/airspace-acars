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
              {perf.airportIcao} · PISTA {perf.runwayId}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-4 text-xs font-medium">
            <span className="flex items-center gap-1.5 text-foreground/90">
              <span className="h-2 w-2 rounded-full bg-emerald-400" />
              {isLanding ? "Toque" : "Ponto de Rotação"}
            </span>
            <span className="flex items-center gap-1.5 text-foreground/90">
              <span className="h-2 w-2 rounded-full bg-amber-400" />
              {isLanding ? "Fim do Rolamento" : "Fim da Decolagem"}
            </span>
          </div>
        </div>

        {/* SVG Runway Canvas strictly matching the Airspace platform design */}
        <div className="relative w-full overflow-hidden pt-1">
          <svg viewBox="0 0 1000 305" className="w-full h-auto block select-none">
            {/* Dimension Line Above: TOQUE (in Emerald Green) */}
            <path d="M 60 56 V 100 M 241 56 V 122 M 60 62 H 241" stroke="#10b981" strokeWidth="1" fill="none" />
            <text x="150" y="56" fill="#10b981" fontSize="11" fontWeight="bold" fontFamily="monospace" textAnchor="middle">
              {isLanding ? `TOQUE ${perf.distanceM} m` : `CORRIDA ${perf.distanceM} m`}
            </text>

            {/* Runway tarmac & white boundaries */}
            <rect x="60" y="100" width="880" height="60" fill="#18181b" stroke="#f4f4f5" strokeWidth="2" rx="1" />

            {/* Threshold stripes at both ends */}
            {[105, 111, 117, 123, 137, 143, 149, 155].map((y) => (
              <g key={y}>
                <rect x="66" y={y} width="18" height="3" fill="#f4f4f5" />
                <rect x="916" y={y} width="18" height="3" fill="#f4f4f5" />
              </g>
            ))}

            {/* Aiming points (thick rectangular markings) */}
            <rect x="180" y="106" width="34" height="7" fill="#a1a1aa" />
            <rect x="180" y="147" width="34" height="7" fill="#a1a1aa" />
            <rect x="786" y="106" width="34" height="7" fill="#a1a1aa" />
            <rect x="786" y="147" width="34" height="7" fill="#a1a1aa" />

            {/* Centerline dashes */}
            <line x1="110" y1="130" x2="890" y2="130" stroke="#71717a" strokeWidth="2.5" strokeDasharray="16 12" />

            {/* Runway orientation numbers */}
            <text x="96" y="134" fill="#e4e4e7" fontSize="14" fontWeight="bold" fontFamily="monospace" textAnchor="middle" transform="rotate(90 96 130)">
              {perf.runwayId}
            </text>
            <text x="904" y="134" fill="#a1a1aa" fontSize="14" fontWeight="bold" fontFamily="monospace" textAnchor="middle" transform="rotate(-90 904 130)">
              {perf.recipId}
            </text>

            {/* Trajectory & Aircraft Track */}
            {isLanding ? (
              <>
                {/* Approach trajectory (dashed cyan) */}
                <path d="M 15 126 C 90 128, 170 120, 241 122" fill="none" stroke="#38bdf8" strokeWidth="2.5" strokeDasharray="4 4" strokeLinecap="round" />
                {/* Rollout track on runway (solid cyan meandering gently) */}
                <path d="M 241 122 C 300 125, 380 137, 460 135 C 530 133, 620 130, 700 130" fill="none" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" />
                {/* Touchdown bullseye marker (Emerald Green) */}
                <circle cx="241" cy="122" r="7" fill="none" stroke="#10b981" strokeWidth="2" />
                <circle cx="241" cy="122" r="3" fill="#10b981" />

                {/* Rollout End bullseye marker (Amber) */}
                <circle cx="700" cy="130" r="7" fill="none" stroke="#f59e0b" strokeWidth="2" />
                <circle cx="700" cy="130" r="3" fill="#f59e0b" />
              </>
            ) : (
              <>
                {/* Takeoff ground roll track */}
                <path d="M 100 130 L 480 128 Q 620 122, 920 85" fill="none" stroke="#38bdf8" strokeWidth="2.5" strokeLinecap="round" />
                {/* Rotation bullseye marker (Emerald Green) */}
                <circle cx="480" cy="128" r="7" fill="none" stroke="#10b981" strokeWidth="2" />
                <circle cx="480" cy="128" r="3" fill="#10b981" />

                {/* Liftoff / End marker (Amber) */}
                <circle cx="750" cy="110" r="7" fill="none" stroke="#f59e0b" strokeWidth="2" />
                <circle cx="750" cy="110" r="3" fill="#f59e0b" />
              </>
            )}

            {/* Dimension Line Below: ROLAMENTO (in Amber) */}
            <path d="M 241 122 V 202 M 700 130 V 202 M 241 195 H 700" stroke="#f59e0b" strokeWidth="1" fill="none" />
            <text x="470" y="189" fill="#f59e0b" fontSize="11" fontWeight="bold" fontFamily="monospace" textAnchor="middle">
              {isLanding ? `ROLAMENTO ${perf.rolloutM} m` : `DECOLAGEM ${perf.runwayUsedM} m`}
            </text>

            {/* Dimension Line Bottom: PISTA dimensions (Muted Gray) */}
            <path d="M 60 236 V 248 M 940 236 V 248 M 60 242 H 940" stroke="#52525b" strokeWidth="1" fill="none" />
            <text x="500" y="237" fill="#a1a1aa" fontSize="11" fontFamily="monospace" textAnchor="middle">
              PISTA {perf.runwayId} · {perf.lengthM.toLocaleString("en-US")} m x {perf.widthM} m
            </text>

            {/* Bottom Caption Notes strictly mirroring the platform */}
            <text x="60" y="286" fill="#71717a" fontSize="11">
              Desvio lateral exagerado 5× para maior clareza — fora de escala. À esquerda da linha central é plotada acima.
            </text>
            <text x="940" y="280" fill="#71717a" fontSize="11" textAnchor="end">
              <tspan x="940" dy="0">PISTA {perf.runwayId} · rumo {perf.heading}°</tspan>
              <tspan x="940" dy="14">Recíproca {perf.recipId}</tspan>
            </text>
          </svg>
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
            {[
              { label: isLanding ? "Taxa de Toque (FPM)" : "Taxa de Subida Inicial", val: `${perf.rateFpm} ft/min`, color: "text-foreground" },
              { label: isLanding ? "G-Force ao Toque" : "G-Force na Rotação", val: `${perf.gForce.toFixed(2)} G`, color: "text-emerald-400" },
              { label: isLanding ? "Distância até o Toque" : "Distância de Corrida", val: `${perf.distanceM} m`, color: "text-foreground" },
              { label: isLanding ? "Distância de Rolamento" : "Distância de Decolagem", val: `${perf.rolloutM} m`, color: "text-amber-400" },
              { label: "Desvio do Eixo Central", val: perf.offsetText, color: "text-foreground" },
              { label: "Ground Speed", val: `${perf.groundSpeedKt} kt`, color: "text-foreground" },
              { label: "Indicated Airspeed", val: `${perf.airSpeedKt} kt`, color: "text-foreground" },
              { label: "Pista Utilizada", val: `${perf.runwayUsedM} m (${perf.runwayUsedPct.toFixed(1)}%)`, color: "text-sky-400" },
            ].map((m, i) => (
              <div key={i} className="flex justify-between py-1.5">
                <span className="text-muted-foreground">{m.label}</span>
                <span className={`font-mono font-semibold ${m.color}`}>{m.val}</span>
              </div>
            ))}
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
            <div className="relative h-28 w-full rounded-lg p-2 border border-border/40 bg-muted/20">
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
