import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { EMPTY_ATTENDANT, MAP_MARKERS } from "@/lib/attendant";
import en from "@/locales/en.json";
import { PhaseMap } from "./phase-map";

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

afterEach(() => { cleanup(); });

interface Box { left: number; right: number; top: number; bottom: number }

// jsdom does no layout, so a label's box is estimated from its attributes:
// the map's 9-11px glyphs average under 6 units wide, rise about 9 above the
// baseline, and the comma in "10,000" drops about 3 below it.
function textBox(el: SVGTextElement): Box {
  const x = Number(el.getAttribute("x"));
  const y = Number(el.getAttribute("y"));
  const width = (el.textContent ?? "").length * 6;
  const left = el.getAttribute("text-anchor") === "middle" ? x - width / 2 : x;
  return { left, right: left + width, top: y - 9, bottom: y + 3 };
}

function svgBox(el: Element): Box {
  const x = Number(el.getAttribute("x"));
  const y = Number(el.getAttribute("y"));
  return { left: x, right: x + Number(el.getAttribute("width")), top: y, bottom: y + Number(el.getAttribute("height")) };
}

const overlaps = (a: Box, b: Box) => a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;

// The aircraft rides the profile line at the last phase fired. Drawn a fixed
// 30 units above its marker, it landed on the name of every airborne phase,
// whose label sits above the line rather than below it.
it.each(MAP_MARKERS.map((m) => [m.key, m] as const))(
  "puts the aircraft on the %s marker without covering any text",
  (key, marker) => {
    render(
      <I18nextProvider i18n={i18n}>
        <PhaseMap snapshot={{ ...EMPTY_ATTENDANT, mode: "client", current: key }} />
      </I18nextProvider>,
    );
    const map = screen.getByRole("group", { name: en["attendant.map"] });
    const icon = within(map).getByRole("img", { name: en["attendant.aircraft"] }).querySelector("svg");
    if (!icon) throw new Error("the aircraft is not drawn");
    const plane = svgBox(icon);

    map.querySelectorAll("text").forEach((label) => {
      expect(overlaps(plane, textBox(label)), `the aircraft at ${key} covers "${label.textContent}"`).toBe(false);
    });
    expect((plane.left + plane.right) / 2, `the aircraft is not over the ${key} marker`).toBe(marker.x);
    expect((plane.top + plane.bottom) / 2, `the aircraft is off the profile line at ${key}`).toBe(marker.y);
  },
);
