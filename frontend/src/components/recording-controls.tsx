import { useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
    <div className="flex items-center gap-3">
      {!isRecording ? (
        <Button
          size="sm"
          onClick={handleStart}
          disabled={!isConnected}
          className="gap-2"
        >
          <Circle className="h-3 w-3 fill-current" />
          {t("recording.startRecording")}
        </Button>
      ) : (
        <Button
          size="sm"
          variant="destructive"
          onClick={handleStop}
          className="gap-2"
        >
          <Square className="h-3 w-3 fill-current" />
          {t("recording.stop")}
        </Button>
      )}

      {isRecording && (
        <>
          <Badge variant="outline" className="gap-1.5 tabular-nums font-mono">
            <span className="h-1.5 w-1.5 rounded-full bg-red-500 animate-pulse" />
            {formatDuration(duration)}
          </Badge>
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("recording.points", { count: dataCount })}
          </span>
        </>
      )}

      <Button
        size="sm"
        variant="outline"
        onClick={handleExport}
        disabled={isRecording}
        className="gap-2 ml-auto"
      >
        <Download className="h-3 w-3" />
        {t("recording.exportCsv")}
      </Button>
    </div>
  );
}
