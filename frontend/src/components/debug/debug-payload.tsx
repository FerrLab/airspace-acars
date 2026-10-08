import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { outcomeKey, secondsSince, type PositionReportSnapshot } from "@/lib/debug-snapshot";
import { SectionTitle } from "./debug-table";

/** The last position report exactly as the backend built it, and its fate. */
export function DebugPayload({ report, now }: { report: PositionReportSnapshot | undefined; now: number }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  if (!report?.json) {
    return <p className="text-sm text-muted-foreground">{t("debug.payload.empty")}</p>;
  }

  function handleCopy() {
    const clipboard = navigator.clipboard;
    if (!clipboard || !report) return;
    clipboard.writeText(report.json).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => {});
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div>
          <SectionTitle>{t("debug.payload.title")}</SectionTitle>
          <p className="text-xs text-muted-foreground">
            {t("debug.payload.meta", {
              outcome: t(outcomeKey(report.outcome)),
              age: secondsSince(report.at, now) ?? 0,
              count: report.batchSize,
            })}
          </p>
        </div>
        <Button variant="outline" size="sm" className="h-7 text-xs" onClick={handleCopy}>
          {copied ? t("debug.copied") : t("debug.copyJson")}
        </Button>
      </div>
      {report.outcome === "preview" && (
        <p className="text-xs text-muted-foreground">{t("debug.payload.previewNote")}</p>
      )}
      <pre className="max-h-[480px] overflow-auto rounded-md border border-border bg-muted/50 p-3 font-mono text-[11px] leading-relaxed">
        {report.json}
      </pre>
    </div>
  );
}
