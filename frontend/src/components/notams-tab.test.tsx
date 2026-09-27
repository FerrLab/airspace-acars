import { act, fireEvent, render, screen, waitFor, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { NOTAMService } from "../../bindings/airspace-acars";
import { NotamsTab } from "./notams-tab";
import en from "@/locales/en.json";
import pt from "@/locales/pt.json";
import es from "@/locales/es.json";
import fr from "@/locales/fr.json";

const auth = vi.hoisted(() => ({ tenant: { id: "a", domain: "a.example", name: "Alpha" }, tokenSynced: true }));
vi.mock("@/context/auth-context", () => ({ useAuth: () => auth }));

const i18n = createInstance();
await i18n.init({ lng: "en", resources: { en: { translation: en } }, keySeparator: false, interpolation: { escapeValue: false } });

function view(localMode = false) {
  return <I18nextProvider i18n={i18n}><NotamsTab localMode={localMode} /></I18nextProvider>;
}
const notice = { id: "42", title: "Runway works", content: "", created_at: "2026-09-26T12:00:00Z" };
const page = { status: "ok", data: [notice], current_page: 1, last_page: 1 };

beforeEach(() => {
  auth.tenant = { id: "a", domain: "a.example", name: "Alpha" };
  vi.spyOn(NOTAMService, "GetNOTAMs").mockResolvedValue(page);
  vi.spyOn(NOTAMService, "GetNOTAM").mockResolvedValue({ status: "ok", data: { ...notice, content: "<p>Use runway 09.</p><script>bad()</script><p>Contact dispatch.</p>" } });
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it("loads the company list and full notice, rendering rich text without executing markup", async () => {
  const { container } = render(view());
  expect(screen.getByText("Active notices from Alpha")).toBeInTheDocument();
  await screen.findByRole("heading", { name: "Runway works" });
  expect(NOTAMService.GetNOTAM).toHaveBeenCalledWith("42");
  expect(screen.getByText(/Use runway 09/)).toHaveTextContent("Contact dispatch.");
  expect(container.querySelector("script")).toBeNull();
  expect(screen.queryByText(/bad\(\)/)).not.toBeInTheDocument();
});

it("shows access denied as an error and allows a manual retry", async () => {
  vi.mocked(NOTAMService.GetNOTAMs).mockResolvedValueOnce({ ...page, status: "accessDenied", data: [] });
  render(view());
  expect(await screen.findByRole("alert")).toHaveTextContent(en["notams.accessDenied"]);
  expect(screen.queryByText(en["notams.empty"])).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByRole("heading", { name: "Runway works" });
});

it("does not call the company API in local mode", () => {
  render(view(true));
  expect(screen.getByText(en["notams.localMode"])).toBeInTheDocument();
  expect(NOTAMService.GetNOTAMs).not.toHaveBeenCalled();
  expect(NOTAMService.GetNOTAM).not.toHaveBeenCalled();
});

it("loads the next page and distinguishes an empty result", async () => {
  vi.mocked(NOTAMService.GetNOTAMs).mockResolvedValueOnce({ ...page, last_page: 2 }).mockResolvedValueOnce({ ...page, data: [], current_page: 2, last_page: 2 });
  render(view());
  fireEvent.click(await screen.findByRole("button", { name: "Next" }));
  await screen.findByText(en["notams.empty"]);
  expect(NOTAMService.GetNOTAMs).toHaveBeenLastCalledWith(2);
  expect(screen.getByText("Page 2 of 2")).toBeInTheDocument();
});

it("discards a previous company's response after switching company", async () => {
  let resolveOld!: (result: typeof page) => void;
  // Runtime tests alias the generated CancellablePromise API to plain promises.
  const pending = new Promise<typeof page>((resolve) => { resolveOld = resolve; });
  vi.mocked(NOTAMService.GetNOTAMs).mockReturnValueOnce(pending as ReturnType<typeof NOTAMService.GetNOTAMs>)
    .mockResolvedValueOnce({ ...page, data: [] });
  const { rerender } = render(view());
  auth.tenant = { id: "b", domain: "b.example", name: "Bravo" };
  rerender(view());
  await screen.findByText(en["notams.empty"]);
  await act(async () => { resolveOld(page); });
  expect(screen.getByText("Active notices from Bravo")).toBeInTheDocument();
  expect(screen.queryByText("Runway works")).not.toBeInTheDocument();
});

it("shows network failure without presenting an empty list", async () => {
  vi.mocked(NOTAMService.GetNOTAMs).mockRejectedValueOnce(new Error("offline"));
  render(view());
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(en["notams.loadError"]));
});

it("includes every NOTAM message and interpolation in all four locales", () => {
  const keys = Object.keys(en).filter((key) => key.startsWith("notams.") || key === "sidebar.notams");
  for (const locale of [pt, es, fr]) {
    for (const key of keys) {
      const value = locale[key as keyof typeof locale];
      expect(value, key).toBeTruthy();
      expect(value.match(/\{\{\w+\}\}/g) ?? []).toEqual(en[key as keyof typeof en].match(/\{\{\w+\}\}/g) ?? []);
    }
  }
});
