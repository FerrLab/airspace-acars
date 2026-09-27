import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Clock3, Globe2 } from "lucide-react";

export function clockReading(now: Date, locale: string, timeZone?: string) {
  return {
    time: new Intl.DateTimeFormat(locale, { timeZone, hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" }).format(now),
    date: new Intl.DateTimeFormat(locale, { timeZone, day: "2-digit", month: "short", year: "numeric" }).format(now),
  };
}

export function FlightClocks() {
  const { t, i18n } = useTranslation();
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    // Read the OS clock every time, rather than incrementing a counter. This
    // catches clock corrections and wake-from-sleep without accumulating drift.
    const update = () => setNow(new Date());
    const timer = window.setInterval(update, 1000);
    window.addEventListener("focus", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, []);

  const zulu = clockReading(now, i18n.language, "UTC");
  // Omitting timeZone uses the computer's timezone, including daylight saving.
  const local = clockReading(now, i18n.language);
  const zone = new Intl.DateTimeFormat(i18n.language, { timeZoneName: "short" }).formatToParts(now).find((part) => part.type === "timeZoneName")?.value;

  return (
    <header className="flex shrink-0 flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border bg-card/60 px-6 py-3" aria-label={t("clock.label")}>
      <div className="hidden items-center gap-2 text-xs text-muted-foreground lg:flex">
        <Clock3 className="h-4 w-4" aria-hidden="true" />{t("clock.source")}
      </div>
      <div className="flex flex-1 flex-wrap items-center justify-end gap-x-7 gap-y-3">
        {[{ key: "zulu", label: t("clock.zulu"), zone: "UTC", reading: zulu, Icon: Globe2 }, { key: "local", label: t("clock.local"), zone, reading: local, Icon: Clock3 }].map(({ key, label, zone: labelZone, reading, Icon }) => (
          <div key={key} className="flex items-center gap-3" title={t("clock.source")}>
            <Icon className="hidden h-4 w-4 text-muted-foreground sm:block" aria-hidden="true" />
            <div>
              <div className="flex items-baseline gap-2">
                <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</span>
                <span className="text-[10px] text-muted-foreground">{labelZone}</span>
              </div>
              <time data-testid={`clock-${key}`} dateTime={now.toISOString()} className="block font-mono text-xl font-medium leading-7 tracking-tight tabular-nums">{reading.time}</time>
              <span className="block text-[10px] text-muted-foreground">{reading.date}</span>
            </div>
          </div>
        ))}
      </div>
    </header>
  );
}
