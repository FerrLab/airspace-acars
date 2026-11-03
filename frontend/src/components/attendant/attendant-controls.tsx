import { useTranslation } from "react-i18next";
import { Music, RotateCcw, SkipForward, Square, Volume2 } from "lucide-react";
import { AttendantService } from "../../../bindings/airspace-acars";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { clipKey, languageVoice, reasonText, stepLabelKey, type AttendantSnapshot } from "@/lib/attendant";

interface Props {
  snapshot: AttendantSnapshot;
  volume: number;
  onVolumeChange: (v: number) => void;
}

/** On/off, volume, skip, replay, stop, and what is playing and why. */
export function AttendantControls({ snapshot, volume, onVolumeChange }: Props) {
  const { t } = useTranslation();
  const clip = snapshot.nowPlaying;
  const queued = snapshot.queue?.length ?? 0;
  const act = (fn: () => Promise<unknown>) => () => { void fn().catch(() => {}); };

  return (
    <section className="space-y-3 rounded-lg border border-border bg-card p-3">
      <div className="flex flex-wrap items-center gap-4">
        <label className="flex items-center gap-2 text-xs font-medium">
          <Switch checked={snapshot.enabled} aria-label={t("attendant.enabled")}
            onCheckedChange={(on) => { void AttendantService.SetEnabled(on).catch(() => {}); }} />
          {t("attendant.enabled")}
        </label>
        <div className="flex min-w-40 flex-1 items-center gap-2 sm:max-w-xs">
          <Volume2 className="h-4 w-4 text-muted-foreground" />
          <Slider min={0} max={100} step={1} value={[volume]} onValueChange={([v]) => onVolumeChange(v)}
            aria-label={t("attendant.volume")} className="flex-1" />
          <span className="w-10 text-right font-mono text-xs tabular-nums">{volume}%</span>
        </div>
        <div className="flex gap-1">
          <Button variant="outline" size="sm" onClick={act(() => AttendantService.Skip())} disabled={!clip}>
            <SkipForward className="h-3.5 w-3.5" />{t("attendant.skip")}
          </Button>
          <Button variant="outline" size="sm" onClick={act(() => AttendantService.Replay())}>
            <RotateCcw className="h-3.5 w-3.5" />{t("attendant.replay")}
          </Button>
          <Button variant="outline" size="sm" onClick={act(() => AttendantService.StopAll())}>
            <Square className="h-3.5 w-3.5" />{t("attendant.stop")}
          </Button>
        </div>
      </div>

      <div className="rounded-md bg-muted/50 p-3" aria-live="polite">
        <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
          <span className="font-semibold uppercase tracking-wide">{t("attendant.nowPlaying")}</span>
          {clip && <Badge className="px-1.5 py-0 text-[10px]">{t(stepLabelKey(clipKey(clip)))}</Badge>}
          {clip && !clip.music && languageVoice(clip) && <span>{languageVoice(clip)}</span>}
          {queued > 0 && <span className="ml-auto">{t("attendant.queued", { count: queued })}</span>}
        </div>
        {clip ? (
          <>
            <p className="mt-1 text-sm">
              {clip.music ? <span className="inline-flex items-center gap-1"><Music className="h-3.5 w-3.5" />{t("attendant.music")}</span> : clip.text}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{reasonText(t, clip.reason)}</p>
          </>
        ) : (
          <p className="mt-1 text-sm text-muted-foreground">{t("attendant.nothingPlaying")}</p>
        )}
      </div>
    </section>
  );
}
