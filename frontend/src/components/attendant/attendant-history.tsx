import { useTranslation } from "react-i18next";
import { clockTime, reasonText, stepLabelKey, type AttendantHistoryEntry } from "@/lib/attendant";

/** What the cabin heard, newest first, with why. */
export function AttendantHistory({ entries }: { entries: AttendantHistoryEntry[] }) {
  const { t } = useTranslation();
  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <h3 className="mb-2 text-xs font-semibold">{t("attendant.history")}</h3>
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("attendant.historyEmpty")}</p>
      ) : (
        <ol className="space-y-1.5">
          {entries.map((e, i) => {
            const key = e.phase === "meal_service" && e.service ? `meal_service:${e.service}` : e.phase;
            return (
              <li key={`${e.at}-${i}`} className="text-xs">
                <div className="flex gap-2">
                  <span className="font-mono tabular-nums text-muted-foreground">{clockTime(e.at)}</span>
                  <span className="font-medium">{t(stepLabelKey(key))}</span>
                  <span className="text-muted-foreground">· {t(`attendant.outcome.${e.outcome}`)}</span>
                </div>
                <div className="truncate">{e.music ? t("attendant.music") : e.text}</div>
                <div className="text-muted-foreground">{reasonText(t, e.reason)}</div>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
