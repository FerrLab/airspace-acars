import { describe, expect, it } from "vitest";
import fs from "fs";
import path from "path";
import en from "./en.json";
import pt from "./pt.json";
import es from "./es.json";
import fr from "./fr.json";

function findDuplicateKeysInJson(jsonText: string): string[] {
  const keyRegex = /"([^"\\]*(?:\\.[^"\\]*)*)"\s*:/g;
  const seen = new Set<string>();
  const duplicates: string[] = [];
  let match: RegExpExecArray | null;
  while ((match = keyRegex.exec(jsonText)) !== null) {
    const key = match[1];
    if (seen.has(key)) {
      duplicates.push(key);
    } else {
      seen.add(key);
    }
  }
  return duplicates;
}

describe("Locale Parity", () => {
  const locales = [
    { name: "pt", data: pt },
    { name: "es", data: es },
    { name: "fr", data: fr },
  ];

  it("ensures all debug keys in en.json exist in pt, es, and fr", () => {
    const enDebugKeys = Object.keys(en).filter((k) => k.startsWith("debug."));
    expect(enDebugKeys.length).toBeGreaterThan(0);
    for (const locale of locales) {
      const missingKeys = enDebugKeys.filter((k) => !(k in locale.data));
      expect(missingKeys, `${locale.name}.json is missing debug keys`).toEqual([]);
    }
  });

  it("ensures all myFlights keys in en.json exist in pt, es, and fr", () => {
    const enFlightKeys = Object.keys(en).filter((k) => k.startsWith("myFlights."));

    expect(enFlightKeys.length).toBeGreaterThan(0);

    for (const locale of locales) {
      const missingKeys = enFlightKeys.filter((k) => !(k in locale.data));
      expect(
        missingKeys,
        `Locale ${locale.name} is missing keys: ${missingKeys.join(", ")}`
      ).toEqual([]);
    }
  });

  it("ensures no duplicate keys in JSON source files", () => {
    const files = ["en.json", "pt.json", "es.json", "fr.json"];
    for (const file of files) {
      const filePath = path.resolve(__dirname, file);
      const text = fs.readFileSync(filePath, "utf-8");
      const duplicates = findDuplicateKeysInJson(text);
      expect(
        duplicates,
        `File ${file} contains duplicate keys: ${duplicates.join(", ")}`
      ).toEqual([]);
    }
  });
});
