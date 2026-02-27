import type { TFunction } from "i18next";

const errorMap: Record<string, { key: string; extract?: RegExp }> = {
  "Failed to connect": { key: "acars.connectFailed", extract: /Failed to connect:\s*(.*)/ },
  "Failed to start flight": { key: "acars.startFlightFailed", extract: /Failed to start flight:\s*(.*)/ },
  "Failed to stop flight": { key: "acars.stopFlightFailed", extract: /Failed to stop flight:\s*(.*)/ },
  "Failed to finish flight": { key: "acars.finishFlightFailed", extract: /Failed to finish flight:\s*(.*)/ },
  "Export failed": { key: "recording.exportFailed", extract: /Export failed:\s*(.*)/ },
};

export function translateError(t: TFunction, raw: string): string {
  const str = String(raw);
  for (const [prefix, { key, extract }] of Object.entries(errorMap)) {
    if (str.startsWith(prefix)) {
      const detail = extract ? (extract.exec(str)?.[1] ?? str) : str;
      return t(key, { error: detail });
    }
  }
  return str;
}
