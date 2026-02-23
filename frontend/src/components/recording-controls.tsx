import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Circle, Square, Download } from "lucide-react";
import { FlightDataService } from "../../bindings/airspace-acars";

interface RecordingControlsProps {
  isRecording: boolean;
  isConnected: boolean;
}

export function RecordingControls({ isRecording, isConnected }: RecordingControlsProps) {
  const [duration, setDuration] = useState(0);
  const [dataCount, setDataCount] = useState(0);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (isRecording) {
      setDuration(0);
      intervalRef.current = setInterval(async () => {
        setDuration((d) => d + 1);
        try {
          const info = await FlightDataService.GetRecordingInfo();
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
      const filePath = prompt("Enter file path for CSV export:", "flight_data.csv");
      if (!filePath) return;
      await FlightDataService.ExportCSV(filePath);
      alert("CSV exported successfully! Database purged.");
    } catch (e: any) {
      console.error("Failed to export CSV:", e);
      alert("Export failed: " + e);
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
          Start Recording
        </Button>
      ) : (
        <Button
          size="sm"
          variant="destructive"
          onClick={handleStop}
          className="gap-2"
        >
          <Square className="h-3 w-3 fill-current" />
          Stop
        </Button>
      )}

      {isRecording && (
        <>
          <Badge variant="outline" className="gap-1.5 tabular-nums font-mono">
            <span className="h-1.5 w-1.5 rounded-full bg-red-500 animate-pulse" />
            {formatDuration(duration)}
          </Badge>
          <span className="text-xs text-muted-foreground tabular-nums">
            {dataCount} points
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
        Export CSV
      </Button>
    </div>
  );
}
