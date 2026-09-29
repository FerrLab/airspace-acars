import { act, fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { DocumentService, FlightService } from "../../bindings/airspace-acars";
import { DocumentsTab, type DocItem } from "./documents-tab";
import en from "@/locales/en.json";

const auth = vi.hoisted(() => ({
  tenant: { id: "test-va", domain: "va.example", name: "Star VA" },
  tokenSynced: true,
}));
vi.mock("@/context/auth-context", () => ({ useAuth: () => auth }));

const i18n = createInstance();
await i18n.init({
  lng: "en",
  resources: { en: { translation: en } },
  keySeparator: false,
  interpolation: { escapeValue: false },
});

function view(localMode = false) {
  return (
    <I18nextProvider i18n={i18n}>
      <DocumentsTab localMode={localMode} />
    </I18nextProvider>
  );
}

const mockDocs: DocItem[] = [
  {
    id: "doc-1",
    title: "Airport Briefing SBGR",
    type: "auto_generated",
    is_auto_generated: true,
    source: "Airport",
    content: "<h3>Runway Incursion Warning</h3><p>Watch for ground traffic at intersection Alpha.</p>",
    sort_order: 1,
    created_at: "2026-09-26T10:00:00Z",
  },
  {
    id: "doc-2",
    title: "B737 FDM Profile",
    type: "auto_generated",
    is_auto_generated: true,
    source: "FDM",
    content: "<p>Pitch limit 18 deg.</p>",
    sort_order: 2,
    created_at: "2026-09-26T10:00:00Z",
  },
  {
    id: "doc-3",
    title: "Standard Operating Procedures (SOP)",
    type: "html",
    is_auto_generated: false,
    content: "<p>Stabilized approach criteria below 1000 ft.</p>",
    sort_order: 3,
    created_at: "2026-09-26T10:00:00Z",
  },
  {
    id: "doc-4",
    title: "Fleet Operations Manual PDF",
    type: "pdf",
    is_auto_generated: false,
    file_url: "https://va.example/files/manual.pdf",
    sort_order: 4,
    created_at: "2026-09-26T10:00:00Z",
  },
];

beforeEach(() => {
  auth.tenant = { id: "test-va", domain: "va.example", name: "Star VA" };
  vi.spyOn(DocumentService, "GetDocuments").mockResolvedValue({
    status: "ok",
    data: mockDocs,
    current_page: 1,
    last_page: 1,
  });
  vi.spyOn(DocumentService, "GetDocument").mockImplementation((id: string) => {
    const found = mockDocs.find((d) => d.id === id);
    return Promise.resolve({
      status: "ok",
      data: found ?? null,
    });
  });
  vi.spyOn(FlightService, "GetActiveFlightInfo").mockResolvedValue({
    departure: "SBGR",
    arrival: "SBRJ",
  });
  vi.spyOn(FlightService, "GetBooking").mockResolvedValue(null);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("loads document list and displays first readable document content", async () => {
  render(view());

  expect(screen.getByText("Documentation")).toBeInTheDocument();
  const titles = await screen.findAllByText("Airport Briefing SBGR");
  expect(titles.length).toBeGreaterThan(0);
  expect(screen.getByText("B737 FDM Profile")).toBeInTheDocument();
  expect(screen.getByText("Standard Operating Procedures (SOP)")).toBeInTheDocument();

  // Selected document content is displayed
  expect(await screen.findByText("Runway Incursion Warning")).toBeInTheDocument();
  expect(screen.getByText(/ground traffic at intersection Alpha/)).toBeInTheDocument();
});

it("filters documents when searching", async () => {
  render(view());

  await screen.findAllByText("Airport Briefing SBGR");
  const searchInput = screen.getByPlaceholderText("Search documents...");

  await act(async () => {
    fireEvent.change(searchInput, { target: { value: "SOP" } });
  });

  expect(screen.getAllByText("Standard Operating Procedures (SOP)").length).toBeGreaterThan(0);
  expect(screen.queryByText("Airport Briefing SBGR")).not.toBeInTheDocument();
});

it("filters by category tabs", async () => {
  render(view());

  await screen.findAllByText("Airport Briefing SBGR");
  const fdmTab = screen.getByRole("button", { name: /FDM Profiles/i });

  await act(async () => {
    fireEvent.click(fdmTab);
  });

  expect(screen.getAllByText("B737 FDM Profile").length).toBeGreaterThan(0);
  expect(screen.queryByText("Airport Briefing SBGR")).not.toBeInTheDocument();
});

it("shows active flight briefing shortcuts", async () => {
  render(view());

  expect(await screen.findByText("Flight Operational Briefing")).toBeInTheDocument();
  const depButton = screen.getByRole("button", { name: "SBGR" });
  expect(depButton).toBeInTheDocument();

  await act(async () => {
    fireEvent.click(depButton);
  });
  const matched = screen.getAllByText("Airport Briefing SBGR");
  expect(matched.length).toBeGreaterThan(0);
  expect(screen.queryByText("B737 FDM Profile")).not.toBeInTheDocument();
});

it("does not call API in local mode", () => {
  render(view(true));

  expect(DocumentService.GetDocuments).not.toHaveBeenCalled();
});

it("intercepts link clicks inside document content and routes through OpenDocumentURL", async () => {
  const docWithLink: DocItem = {
    id: "doc-link",
    title: "Document with Link",
    type: "html",
    is_auto_generated: false,
    content: '<p>See <a href="https://airspace.ferrlab.com/sop/b737">Flight Manual</a> for details.</p>',
    sort_order: 1,
  };
  vi.spyOn(DocumentService, "GetDocuments").mockResolvedValue({
    status: "ok",
    data: [docWithLink],
    current_page: 1,
    last_page: 1,
  });
  vi.spyOn(DocumentService, "GetDocument").mockResolvedValue({
    status: "ok",
    data: docWithLink,
  });
  const openUrlSpy = vi.spyOn(DocumentService, "OpenDocumentURL").mockResolvedValue();

  render(view());

  const link = await screen.findByRole("link", { name: "Flight Manual" });
  expect(link).toBeInTheDocument();

  await act(async () => {
    fireEvent.click(link);
  });

  expect(openUrlSpy).toHaveBeenCalledWith("https://airspace.ferrlab.com/sop/b737");
});

it("pages through to last_page when multiple pages exist", async () => {
  const page1Doc: DocItem = {
    id: "doc-p1",
    title: "Page 1 Document",
    type: "html",
    is_auto_generated: false,
    content: "<p>Page 1 content</p>",
    sort_order: 1,
  };
  const page2Doc: DocItem = {
    id: "doc-p2",
    title: "Page 2 Document",
    type: "html",
    is_auto_generated: false,
    content: "<p>Page 2 content</p>",
    sort_order: 2,
  };

  const getDocsSpy = vi.spyOn(DocumentService, "GetDocuments").mockImplementation((page: number) => {
    if (page === 1) {
      return Promise.resolve({
        status: "ok",
        data: [page1Doc],
        current_page: 1,
        last_page: 2,
      });
    }
    return Promise.resolve({
      status: "ok",
      data: [page2Doc],
      current_page: 2,
      last_page: 2,
    });
  });

  render(view());

  expect(await screen.findByText("Page 1 Document")).toBeInTheDocument();
  expect(await screen.findByText("Page 2 Document")).toBeInTheDocument();
  expect(getDocsSpy).toHaveBeenCalledWith(1, "", "");
  expect(getDocsSpy).toHaveBeenCalledWith(2, "", "");
});

