import { useTranslation } from "react-i18next";
import { useAttendant } from "@/hooks/use-attendant";
import { AttendantControls } from "@/components/attendant/attendant-controls";
import { AttendantHistory } from "@/components/attendant/attendant-history";
import { CabinStatus } from "@/components/attendant/cabin-status";
import { PhaseList } from "@/components/attendant/phase-list";
import { PhaseMap } from "@/components/attendant/phase-map";

interface AttendantTabProps {
  volume: number;
  onVolumeChange: (v: number) => void;
}

/** The Flight Attendant: what the cabin hears, when, and why. */
export function AttendantTab({ volume, onVolumeChange }: AttendantTabProps) {
  const { t } = useTranslation();
  const snapshot = useAttendant();

  return (
    <div className="space-y-4">
      <header>
        <h2 className="text-lg font-semibold tracking-tight">{t("attendant.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("attendant.subtitle")}</p>
      </header>

      <div role="status" className="rounded-md border border-border/60 px-3 py-2 text-xs">
        {!snapshot.enabled
          ? t("attendant.disabled")
          : snapshot.preparing
            ? t("attendant.preparing", { ready: snapshot.preparing.linesReady, total: snapshot.preparing.linesTotal })
            : t(`attendant.mode.${snapshot.mode}`)}
        {snapshot.error && <span className="block text-destructive">{t("attendant.loadError", { error: snapshot.error })}</span>}
      </div>

      <AttendantControls snapshot={snapshot} volume={volume} onVolumeChange={onVolumeChange} />
      <PhaseMap snapshot={snapshot} />
      <div className="grid gap-4 md:grid-cols-2">
        <PhaseList snapshot={snapshot} />
        <div className="space-y-4">
          <CabinStatus snapshot={snapshot} />
          <AttendantHistory entries={snapshot.history ?? []} />
        </div>
      </div>
    </div>
  );
}
