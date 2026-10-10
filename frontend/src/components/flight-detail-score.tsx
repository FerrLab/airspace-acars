import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Award, ChevronDown, ChevronRight, HelpCircle } from "lucide-react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import {
  formatScoreTier,
  getScoreCategories,
  type ScoreCategory,
} from "@/lib/flight-detail";

interface ScoreProps {
  flight: FlightLog;
}

export function FlightDetailScore({ flight }: ScoreProps) {
  const { t } = useTranslation();
  const rawScore = flight.score > 0 ? (flight.score > 10 ? flight.score / 10 : flight.score) : 9.2;
  const scoreCategories: ScoreCategory[] = getScoreCategories(flight);
  const tier = formatScoreTier(rawScore * 10);

  // Expanded categories for "How it's scored" formula
  const [expandedKeys, setExpandedKeys] = useState<Record<string, boolean>>({});

  const toggleExpand = (key: string) => {
    setExpandedKeys((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  // SVG Radial Gauge calculation
  const radius = 42;
  const circumference = 2 * Math.PI * radius;
  const scorePercent = Math.min(100, Math.max(0, rawScore * 10));
  const strokeDashoffset = circumference - (scorePercent / 100) * circumference;

  return (
    <div className="space-y-4">
      {/* Composite Score Hero Banner */}
      <Card className="border-border/70 bg-card p-5 shadow-xs">
        <div className="flex flex-wrap items-center gap-6">
          {/* Circular Score Gauge */}
          <div className="relative w-28 h-28 shrink-0 flex items-center justify-center">
            <svg viewBox="0 0 100 100" className="w-full h-full -rotate-90">
              <circle
                cx="50"
                cy="50"
                r={radius}
                fill="none"
                stroke="rgba(255,255,255,0.08)"
                strokeWidth="6"
              />
              <circle
                cx="50"
                cy="50"
                r={radius}
                fill="none"
                stroke={tier.color}
                strokeWidth="6"
                strokeDasharray={circumference}
                strokeDashoffset={strokeDashoffset}
                strokeLinecap="round"
                className="transition-all duration-700 ease-out"
              />
            </svg>
            <div className="absolute inset-0 flex flex-col items-center justify-center text-center">
              <span className="font-mono text-2xl font-bold leading-none text-foreground" style={{ color: tier.color }}>
                {rawScore.toFixed(2)}
              </span>
              <span className="font-mono text-[10px] text-muted-foreground mt-0.5">/ 10</span>
            </div>
          </div>

          {/* Description & Weighted Bar */}
          <div className="flex-1 min-w-[240px] space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-base font-bold text-foreground">
                {t("myFlights.compositeScore", "Score Composto")}
              </span>
              <Badge variant="outline" className="text-xs font-semibold px-2 border-amber-500/40 text-amber-400 bg-amber-500/10">
                <Award className="h-3 w-3 mr-1" />
                {tier.tier} Tier
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">
              Média ponderada entre categorias operacionais do voo (pontualidade, eficiência de combustível, conforto e segurança).
            </p>

            {/* Segmented Progress Bar */}
            <div className="flex h-2.5 w-full gap-1 rounded-full overflow-hidden bg-muted/20 p-0.5">
              {scoreCategories.map((c) => (
                <div
                  key={c.key}
                  style={{ width: `${c.weight}%`, backgroundColor: c.color }}
                  className="h-full rounded-sm opacity-90"
                  title={`${c.defaultLabel} (${c.weight}%)`}
                />
              ))}
            </div>

            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground pt-1">
              {scoreCategories.map((c) => (
                <span key={c.key} className="flex items-center gap-1">
                  <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: c.color }} />
                  {c.defaultLabel} ({c.score.toFixed(1)})
                </span>
              ))}
            </div>
          </div>
        </div>
      </Card>

      {/* Categories Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {scoreCategories.map((cat) => (
          <Card key={cat.key} className="border-border/70 bg-card p-4 shadow-xs space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold text-foreground flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ backgroundColor: cat.color }} />
                {cat.defaultLabel}
              </span>
              <span className="font-mono text-sm font-bold" style={{ color: cat.color }}>
                {cat.score.toFixed(2)} <span className="text-[10px] text-muted-foreground">/ 10</span>
              </span>
            </div>

            {/* Category Progress */}
            <div className="h-1.5 w-full rounded-full bg-muted/20 overflow-hidden">
              <div
                className="h-full rounded-full"
                style={{ width: `${cat.score * 10}%`, backgroundColor: cat.color }}
              />
            </div>

            {/* Sub-inputs Table */}
            <div className="divide-y divide-border/40 text-[11px]">
              {cat.inputs.map(([label, val], idx) => (
                <div key={idx} className="flex justify-between py-1.5">
                  <span className="text-muted-foreground">{label}</span>
                  <span className="font-mono font-medium text-foreground">{val}</span>
                </div>
              ))}
            </div>

            {/* Formula Accordion */}
            <div className="pt-1">
              <button
                type="button"
                onClick={() => toggleExpand(cat.key)}
                className="flex items-center gap-1 text-[11px] font-medium text-muted-foreground hover:text-foreground transition-colors"
              >
                {expandedKeys[cat.key] ? (
                  <ChevronDown className="h-3 w-3" />
                ) : (
                  <ChevronRight className="h-3 w-3" />
                )}
                <HelpCircle className="h-3 w-3" />
                <span>{t("myFlights.howItsScored", "Como é calculado")}</span>
              </button>

              {expandedKeys[cat.key] && (
                <div className="mt-2 rounded bg-zinc-950 p-2 border border-border/40 text-[10px] font-mono text-muted-foreground leading-relaxed">
                  {cat.formula}
                </div>
              )}
            </div>
          </Card>
        ))}
      </div>

      {/* Flight Points Ledger */}
      <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            {t("myFlights.flightPoints", "Pontos de Voo")}
          </span>
          <Badge variant="outline" className="text-xs font-mono font-bold border-emerald-500/40 text-emerald-400 bg-emerald-500/10">
            +100 pts
          </Badge>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-border/60 text-muted-foreground text-[10px] uppercase font-semibold">
                <th className="py-2 pr-3">Origem</th>
                <th className="py-2 pr-3">Motivo</th>
                <th className="py-2 pr-3 text-right">Pontos</th>
                <th className="py-2 pr-3">Observações</th>
                <th className="py-2">Data</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/40">
              <tr>
                <td className="py-2.5 pr-3">
                  <Badge variant="outline" className="text-[9px] px-1.5 py-0 border-sky-500/30 text-sky-400 bg-sky-500/10">FDM</Badge>
                </td>
                <td className="py-2.5 pr-3 text-foreground font-medium">Approach Stabilized at 500 ft AFL</td>
                <td className="py-2.5 pr-3 text-right font-mono font-bold text-emerald-400">+20</td>
                <td className="py-2.5 pr-3 text-muted-foreground text-[11px]">Critérios de estabilização cumpridos</td>
                <td className="py-2.5 font-mono text-muted-foreground text-[11px]">2026-10-08</td>
              </tr>
              <tr>
                <td className="py-2.5 pr-3">
                  <Badge variant="outline" className="text-[9px] px-1.5 py-0 border-sky-500/30 text-sky-400 bg-sky-500/10">FDM</Badge>
                </td>
                <td className="py-2.5 pr-3 text-foreground font-medium">Flight Time: 1-2 hours</td>
                <td className="py-2.5 pr-3 text-right font-mono font-bold text-emerald-400">+60</td>
                <td className="py-2.5 pr-3 text-muted-foreground text-[11px]">Tempo total de 80 min</td>
                <td className="py-2.5 font-mono text-muted-foreground text-[11px]">2026-10-08</td>
              </tr>
              <tr>
                <td className="py-2.5 pr-3">
                  <Badge variant="outline" className="text-[9px] px-1.5 py-0 border-sky-500/30 text-sky-400 bg-sky-500/10">FDM</Badge>
                </td>
                <td className="py-2.5 pr-3 text-foreground font-medium">Landing G-Force: Smooth (&lt; 1.2 G)</td>
                <td className="py-2.5 pr-3 text-right font-mono font-bold text-emerald-400">+20</td>
                <td className="py-2.5 pr-3 text-muted-foreground text-[11px]">Toque suave a 1.16 G</td>
                <td className="py-2.5 font-mono text-muted-foreground text-[11px]">2026-10-08</td>
              </tr>
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
