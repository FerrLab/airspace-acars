import { useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Circle, Square, Download } from "lucide-react";
import { FlightDataService } from "../../bindings/airspace-acars";

interface RecordingControlsProps {
  isRecording: boolean;
  isConnected: boolean;
}

export function RecordingControls({ isRecording, isConnected }: RecordingControlsProps) {
  const { t } = useTranslation();
  const [duration, setDuration] = useState(0);
  const [dataCount, setDataCount] = useState(0);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (isRecording) {
      // Fetch actual elapsed time from backend (survives tab switches)
      FlightDataService.GetRecordingInfo().then((info) => {
        setDuration(Math.floor(info.duration as number));
        setDataCount(info.dataCount as number);
      }).catch(() => {});

      intervalRef.current = setInterval(async () => {
        try {
          const info = await FlightDataService.GetRecordingInfo();
          setDuration(Math.floor(info.duration as number));
          setDataCount(info.dataCount as number);
        } catch { /* ignore */ }
      }, 1000);
    } else {
      if (intervalRef.current) clearInterval(intervalRef.current);
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [isRecording]);

  const handleStart = async () => {
    try {
      await FlightDataService.StartRecording();
    } catch (e: any) {
      console.error("Failed to start recording:", e);
    }
  };

  const handleStop = async () => {
    try {
      FlightDataService.StopRecording();
    } catch (e: any) {
      console.error("Failed to stop recording:", e);
    }
  };

  const handleExport = async () => {
    try {
      const filePath = prompt(t("recording.exportPrompt"), "flight_data.csv");
      if (!filePath) return;
      await FlightDataService.ExportCSV(filePath);
      alert(t("recording.exportSuccess"));
    } catch (e: any) {
      console.error("Failed to export CSV:", e);
      alert(t("recording.exportFailed", { error: String(e) }));
    }
  };

  const formatDuration = (secs: number) => {
    const m = Math.floor(secs / 60).toString().padStart(2, "0");
    const s = (secs % 60).toString().padStart(2, "0");
    return `${m}:${s}`;
  };

  return (
    <Card className="gap-0 flex-row flex-wrap items-center justify-between gap-3 px-4 py-2.5 border-border/80 bg-card shadow-xs">
      <div className="flex items-center gap-3">
        {!isRecording ? (
          <Button
            size="sm"
            variant="outline"
            onClick={handleStart}
            disabled={!isConnected}
            className="gap-2 shadow-xs"
          >
            <Circle className="h-2.5 w-2.5 fill-emerald-500 text-emerald-500" />
            {t("recording.startRecording")}
          </Button>
        ) : (
          <Button
            size="sm"
            variant="destructive"
            onClick={handleStop}
            className="gap-2 shadow-xs"
          >
            <Square className="h-3 w-3 fill-current" />
            {t("recording.stop")}
          </Button>
        )}

        {isRecording && (
          <>
            <Badge variant="outline" className="gap-1.5 tabular-nums font-mono border-destructive/30 bg-destructive/10 text-destructive">
              <span className="h-1.5 w-1.5 rounded-full bg-destructive animate-pulse" />
              {formatDuration(duration)}
            </Badge>
            <span className="text-xs font-medium text-muted-foreground tabular-nums">
              {t("recording.points", { count: dataCount })}
            </span>
          </>
        )}
      </div>

      <Button
        size="sm"
        variant="outline"
        onClick={handleExport}
        disabled={isRecording}
        className="gap-2 shadow-xs ml-auto"
      >
        <Download className="h-3.5 w-3.5" />
        {t("recording.exportCsv")}
      </Button>
    </Card>
  );
}
