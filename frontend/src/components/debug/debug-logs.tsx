import { useState, useEffect, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { UpdateService } from "../../../bindings/airspace-acars";

function tailLogs(n: number): Promise<string[]> {
  return UpdateService.TailLogs(n);
}

interface LogEntry {
  time?: string;
  level?: string;
  msg?: string;
  [key: string]: unknown;
}

function parseLogLine(line: string): LogEntry | null {
  try {
    return JSON.parse(line);
  } catch {
    return null;
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString([], { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return iso;
  }
}

const levelColors: Record<string, string> = {
  DEBUG: "text-zinc-500",
  INFO: "text-blue-400",
  WARN: "text-yellow-400",
  ERROR: "text-red-400",
};

/** The app's own log, tailed every two seconds. */
export function DebugLogs() {
  const { t } = useTranslation();
  const [lines, setLines] = useState<string[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [paused, setPaused] = useState(false);
  const containerRef = useRef<HTMLPreElement>(null);

  const fetchLogs = useCallback(() => {
    if (paused) return;
    tailLogs(200)
      .then(setLines)
      .catch(() => {});
  }, [paused]);

  useEffect(() => {
    fetchLogs();
    const id = setInterval(fetchLogs, 2000);
    return () => clearInterval(id);
  }, [fetchLogs]);

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [lines, autoScroll]);

  function handleScroll() {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40);
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("debug.logs")}
        </h3>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs"
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? t("debug.logsResume") : t("debug.logsPause")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs"
            onClick={fetchLogs}
          >
            {t("debug.logsRefresh")}
          </Button>
        </div>
      </div>
      <pre
        ref={containerRef}
        onScroll={handleScroll}
        className="rounded-md border border-border bg-zinc-950 p-3 text-[11px] font-mono leading-relaxed overflow-auto h-[300px]"
      >
        {lines.length === 0 && (
          <span className="text-muted-foreground">{t("debug.logsEmpty")}</span>
        )}
        {lines.map((line, i) => {
          const entry = parseLogLine(line);
          if (!entry) {
            return <div key={i} className="text-zinc-400">{line}</div>;
          }
          const lvl = (entry.level || "INFO").toUpperCase();
          const colorClass = levelColors[lvl] || "text-zinc-400";
          const time = entry.time ? formatTime(entry.time) : "";
          // Show remaining fields as key=value
          const extra = Object.entries(entry)
            .filter(([k]) => !["time", "level", "msg"].includes(k))
            .map(([k, v]) => `${k}=${typeof v === "object" ? JSON.stringify(v) : v}`)
            .join(" ");
          return (
            <div key={i} className="hover:bg-zinc-900/50">
              <span className="text-zinc-600">{time}</span>
              {" "}
              <span className={colorClass}>{lvl.padEnd(5)}</span>
              {" "}
              <span className="text-zinc-300">{entry.msg}</span>
              {extra && <span className="text-zinc-600"> {extra}</span>}
            </div>
          );
        })}
      </pre>
    </div>
  );
}
