import { useTranslation } from "react-i18next";
import { gsxStateKey, type AttendantSnapshot } from "@/lib/attendant";

/** What the attendant reads from GSX and the cabin. */
export function CabinStatus({ snapshot }: { snapshot: AttendantSnapshot }) {
  const { t } = useTranslation();
  const { gsx, seatbelt } = snapshot;
  const sign = !seatbelt.seatbeltKnown ? "unknown" : seatbelt.seatbeltSign ? "on" : "off";
  return (
    <section className="rounded-lg border border-border bg-card p-3 text-xs">
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        <dt className="text-muted-foreground">{t("attendant.gsx.title")}</dt>
        <dd>{gsx.connected ? t("attendant.gsx.connected") : t("attendant.gsx.disconnected")}</dd>
        {gsx.connected && (
          <>
            <dt className="text-muted-foreground">{t("attendant.gsx.boarding")}</dt>
            <dd>
              {t(gsxStateKey(gsx.boarding.state))}
              {gsx.boarding.progress && <span className="text-muted-foreground"> · {gsx.boarding.progress}</span>}
            </dd>
          </>
        )}
        <dt className="text-muted-foreground">{t("attendant.seatbeltSign")}</dt>
        <dd>{t(`attendant.seatbelt.${sign}`)}</dd>
      </dl>
    </section>
  );
}
