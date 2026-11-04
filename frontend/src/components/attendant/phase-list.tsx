import { useTranslation } from "react-i18next";
import { Play } from "lucide-react";
import { AttendantService } from "../../../bindings/airspace-acars";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  clipKey, clockTime, markerState, reasonText, stepLabelKey, stepsOf, type AttendantSnapshot,
} from "@/lib/attendant";

/** Every phase with its lines, when it fired and why. */
export function PhaseList({ snapshot }: { snapshot: AttendantSnapshot }) {
  const { t } = useTranslation();
  const playing = clipKey(snapshot.nowPlaying);
  const canPlay = snapshot.mode === "client" && snapshot.enabled;

  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <h3 className="mb-2 text-xs font-semibold">{t("attendant.phases")}</h3>
      <ul className="divide-y divide-border/60">
        {stepsOf(snapshot).map((step) => {
          const state = markerState(step, playing);
          const fired = clockTime(step.firedAt);
          return (
            <li key={step.key} className="flex items-start gap-2 py-1.5 text-xs" aria-label={t(stepLabelKey(step.key))}>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{t(stepLabelKey(step.key))}</span>
                  <Badge variant={state === "fired" || state === "playing" ? "default" : "outline"} className="px-1.5 py-0 text-[10px]">
                    {t(`attendant.status.${state}`)}
                  </Badge>
                  {step.count > 1 && <span className="text-muted-foreground">×{step.count}</span>}
                </div>
                <div className="text-muted-foreground">
                  {step.lines >= 0 && (step.lines === 0 ? t("attendant.noLines") : t("attendant.linesCount", { count: step.lines }))}
                  {fired && <> · {fired}</>}
                  {step.reason?.code && <> · {reasonText(t, step.reason)}</>}
                </div>
              </div>
              {canPlay && step.lines > 0 && (
                <Button variant="ghost" size="icon-sm" aria-label={`${t("attendant.playNow")}: ${t(stepLabelKey(step.key))}`}
                  onClick={() => { void AttendantService.PlayPhase(step.phase, step.service ?? 0).catch(() => {}); }}>
                  <Play className="h-3.5 w-3.5" />
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
