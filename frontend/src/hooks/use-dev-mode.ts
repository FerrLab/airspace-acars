import { useState, useEffect } from "react";
import { UpdateService } from "../../bindings/airspace-acars";

let cached: boolean | null = null;

export function useDevMode(): boolean {
  const [devMode, setDevMode] = useState(cached ?? false);

  useEffect(() => {
    if (cached !== null) return;
    UpdateService.GetCurrentVersion().then((v) => {
      cached = v === "dev" || v.includes("-beta");
      setDevMode(cached);
    }).catch(() => {});
  }, []);

  return devMode;
}
