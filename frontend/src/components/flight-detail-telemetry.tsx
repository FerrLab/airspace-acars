import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Activity } from "lucide-react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Card } from "@/components/ui/card";
import {
  generateFlightTelemetry,
  type TelemetrySample,
} from "@/lib/flight-detail";

interface TelemetryProps {
  flight: FlightLog;
}

export function FlightDetailTelemetry({ flight }: TelemetryProps) {
  const { t } = useTranslation();
  const samples = generateFlightTelemetry(flight);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

  // Active track toggles
  const [showAlt, setShowAlt] = useState(true);
  const [showSpeed, setShowSpeed] = useState(true);
  const [showN1, setShowN1] = useState(false);
  const [showFuel, setShowFuel] = useState(true);

  const activeIndex = hoverIndex != null ? hoverIndex : Math.floor(samples.length * 0.85);
  const currentSample: TelemetrySample = samples[activeIndex] || samples[0];

  // SVG dimensions
  const W = 800;
  const H = 220;

  // Max values for normalization
  const maxAlt = 38000;
  const maxSpeed = 500;
  const maxFuel = samples[0]?.fuel || 6000;

  // Path generators
  const altPath = samples
    .map((s, idx) => {
      const x = (idx / (samples.length - 1)) * W;
      const y = H - (s.alt / maxAlt) * (H - 20) - 10;
      return `${idx === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

  const gsPath = samples
    .map((s, idx) => {
      const x = (idx / (samples.length - 1)) * W;
      const y = H - (s.gs / maxSpeed) * (H - 20) - 10;
      return `${idx === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

  const iasPath = samples
    .map((s, idx) => {
      const x = (idx / (samples.length - 1)) * W;
      const y = H - (s.ias / maxSpeed) * (H - 20) - 10;
      return `${idx === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

  const fuelPath = samples
    .map((s, idx) => {
      const x = (idx / (samples.length - 1)) * W;
      const y = H - (s.fuel / maxFuel) * (H - 20) - 10;
      return `${idx === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const ratio = Math.max(0, Math.min(1, x / rect.width));
    const idx = Math.round(ratio * (samples.length - 1));
    setHoverIndex(idx);
  };

  const cursorXPercent = (activeIndex / (samples.length - 1)) * 100;

  return (
    <Card className="border-border/70 bg-card p-5 shadow-xs space-y-4">
      {/* Header and Filter Chips */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block flex items-center gap-1.5">
            <Activity className="h-3.5 w-3.5 text-primary" />
            {t("myFlights.flightDataTracks", "Dados de Voo · Traçados Sincronizados")}
          </span>
          <span className="text-xs text-muted-foreground">
            {hoverIndex != null
              ? `T+${currentSample.t} min · ${currentSample.phase}`
              : "Passe o cursor sobre o gráfico para inspecionar pontos"}
          </span>
        </div>

        {/* Chips */}
        <div className="flex flex-wrap gap-2 text-xs">
          <button
            type="button"
            onClick={() => setShowAlt(!showAlt)}
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full border transition-colors ${
              showAlt
                ? "bg-sky-500/10 text-sky-400 border-sky-500/40"
                : "bg-muted/10 text-muted-foreground border-border/40"
            }`}
          >
            <span className="h-2 w-2 rounded-full bg-sky-400" />
            Altitude
          </button>
          <button
            type="button"
            onClick={() => setShowSpeed(!showSpeed)}
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full border transition-colors ${
              showSpeed
                ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/40"
                : "bg-muted/10 text-muted-foreground border-border/40"
            }`}
          >
            <span className="h-2 w-2 rounded-full bg-emerald-400" />
            Ground Speed & IAS
          </button>
          <button
            type="button"
            onClick={() => setShowFuel(!showFuel)}
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full border transition-colors ${
              showFuel
                ? "bg-amber-500/10 text-amber-400 border-amber-500/40"
                : "bg-muted/10 text-muted-foreground border-border/40"
            }`}
          >
            <span className="h-2 w-2 rounded-full bg-amber-400" />
            Fuel
          </button>
          <button
            type="button"
            onClick={() => setShowN1(!showN1)}
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full border transition-colors ${
              showN1
                ? "bg-purple-500/10 text-purple-400 border-purple-500/40"
                : "bg-muted/10 text-muted-foreground border-border/40"
            }`}
          >
            <span className="h-2 w-2 rounded-full bg-purple-400" />
            Engine N1
          </button>
        </div>
      </div>

      {/* Synchronized Telemetry Interactive Canvas */}
      <div
        className="relative w-full rounded-lg bg-zinc-950 p-3 border border-border/50 cursor-crosshair overflow-hidden"
        onMouseMove={handleMouseMove}
        onMouseLeave={() => setHoverIndex(null)}
      >
        {/* Phase Header Bar */}
        <div className="flex h-5 w-full border-b border-border/30 text-[9px] font-semibold font-mono text-muted-foreground mb-2">
          <div className="w-[10%] border-r border-border/30 flex items-center justify-center bg-muted/5">TAXI</div>
          <div className="w-[12%] border-r border-border/30 flex items-center justify-center bg-sky-500/5">TAKEOFF</div>
          <div className="w-[20%] border-r border-border/30 flex items-center justify-center bg-sky-500/10">CLIMB</div>
          <div className="w-[30%] border-r border-border/30 flex items-center justify-center bg-emerald-500/10">CRUISE</div>
          <div className="w-[15%] border-r border-border/30 flex items-center justify-center bg-amber-500/10">DESCENT</div>
          <div className="w-[8%] border-r border-border/30 flex items-center justify-center bg-rose-500/10">APPR</div>
          <div className="w-[5%] flex items-center justify-center bg-purple-500/10">LDG</div>
        </div>

        {/* SVG Graphic Curves */}
        <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="w-full h-44 block select-none">
          {/* Horizontal grid lines */}
          <line x1="0" y1={H * 0.25} x2={W} y2={H * 0.25} stroke="rgba(255,255,255,0.06)" />
          <line x1="0" y1={H * 0.5} x2={W} y2={H * 0.5} stroke="rgba(255,255,255,0.06)" />
          <line x1="0" y1={H * 0.75} x2={W} y2={H * 0.75} stroke="rgba(255,255,255,0.06)" />

          {/* Altitude Curve */}
          {showAlt && (
            <>
              <path d={`${altPath} L ${W} ${H} L 0 ${H} Z`} fill="#38bdf8" opacity="0.08" />
              <path d={altPath} fill="none" stroke="#38bdf8" strokeWidth="2" strokeLinejoin="round" />
            </>
          )}

          {/* Speed Curves */}
          {showSpeed && (
            <>
              <path d={gsPath} fill="none" stroke="#10b981" strokeWidth="2" strokeLinejoin="round" />
              <path d={iasPath} fill="none" stroke="#34d399" strokeWidth="1.5" strokeDasharray="4 3" />
            </>
          )}

          {/* Fuel Curve */}
          {showFuel && (
            <path d={fuelPath} fill="none" stroke="#f59e0b" strokeWidth="1.75" strokeLinejoin="round" />
          )}

          {/* Touchdown Marker Pin */}
          <line x1={W * 0.95} y1="0" x2={W * 0.95} y2={H} stroke="#ef4444" strokeDasharray="3 3" opacity="0.6" />
        </svg>

        {/* Interactive Scrub Line */}
        <div
          className="absolute top-8 bottom-3 pointer-events-none border-l-2 border-white/70"
          style={{ left: `${cursorXPercent}%` }}
        />

        {/* Current Readout Overlay Badge */}
        <div
          className="absolute bottom-2 pointer-events-none transform -translate-x-1/2 bg-zinc-900/90 backdrop-blur border border-border/80 px-2 py-0.5 rounded text-[10px] font-mono font-bold text-foreground"
          style={{ left: `${cursorXPercent}%` }}
        >
          {currentSample.alt.toLocaleString()} ft · {currentSample.gs} kt
        </div>
      </div>

      {/* Telemetry Metrics Readout Cards Grid */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
        <div className="rounded-lg bg-muted/10 p-2.5 border border-border/40">
          <span className="text-muted-foreground block text-[10px]">Altitude Atual</span>
          <span className="font-mono text-base font-bold text-sky-400">{currentSample.alt.toLocaleString()} ft</span>
          <span className="text-[10px] text-muted-foreground block">FL{Math.round(currentSample.alt / 100)}</span>
        </div>
        <div className="rounded-lg bg-muted/10 p-2.5 border border-border/40">
          <span className="text-muted-foreground block text-[10px]">Ground Speed / IAS</span>
          <span className="font-mono text-base font-bold text-emerald-400">{currentSample.gs} / {currentSample.ias} kt</span>
          <span className="text-[10px] text-muted-foreground block">{currentSample.phase} Phase</span>
        </div>
        <div className="rounded-lg bg-muted/10 p-2.5 border border-border/40">
          <span className="text-muted-foreground block text-[10px]">Combustível Restante</span>
          <span className="font-mono text-base font-bold text-amber-400">{currentSample.fuel.toLocaleString()} kg</span>
          <span className="text-[10px] text-muted-foreground block">Trip Remaining</span>
        </div>
        <div className="rounded-lg bg-muted/10 p-2.5 border border-border/40">
          <span className="text-muted-foreground block text-[10px]">Configuração da Aeronave</span>
          <span className="font-mono text-base font-bold text-foreground">
            Flaps {currentSample.flaps} · Gear {currentSample.gear ? "Down" : "Up"}
          </span>
          <span className="text-[10px] text-muted-foreground block">Pitch: {currentSample.pitch}°</span>
        </div>
      </div>
    </Card>
  );
}
