import { useEffect, useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertCircle,
  BookOpen,
  Calendar,
  ChevronRight,
  FileText,
  Folder,
  FolderOpen,
  Layers,
  Loader2,
  Plane,
  RefreshCw,
  Search,
  ShieldAlert,
  Sparkles,
  X,
} from "lucide-react";
import { DocumentService, FlightService } from "../../bindings/airspace-acars";
import { useAuth } from "@/context/auth-context";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { sanitizeDocumentHtml } from "@/lib/document-sanitize";

export interface DocItem {
  id: string;
  title: string;
  type: string; // "html" | "pdf" | "auto_generated" | "folder"
  parent_id?: string;
  visibility?: string;
  is_auto_generated: boolean;
  source?: string;
  /** Stable key for auto-generated documents: airport, load_profile, fdm_profile. */
  source_type?: string;
  content?: string;
  file_url?: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
}

const knownStatuses = ["accessDenied", "unavailable", "rateLimited", "noSession", "localMode"];
function statusKey(status: string) {
  return `documents.${knownStatuses.includes(status) ? status : "loadError"}`;
}

export function DocumentsTab({ localMode = false }: { localMode?: boolean }) {
  const { tenant, tokenSynced } = useAuth();
  return (
    <CompanyDocuments
      key={`${tenant?.id}:${tenant?.domain}`}
      company={tenant?.name ?? ""}
      localMode={localMode}
      ready={tokenSynced && !!tenant}
    />
  );
}

function CompanyDocuments({
  company,
  localMode,
  ready,
}: {
  company: string;
  localMode: boolean;
  ready: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [documents, setDocuments] = useState<DocItem[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState<string>("all");
  const [refresh, setRefresh] = useState(0);

  // Active flight context for intelligent airport briefing shortcuts
  const [flightBriefing, setFlightBriefing] = useState<{ departure?: string; arrival?: string } | null>(null);

  // Detail loading state
  const [detailDoc, setDetailDoc] = useState<DocItem | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");

  // Check active booking / flight route for quick briefing shortcuts
  useEffect(() => {
    if (localMode || !ready) return;
    FlightService.GetActiveFlightInfo()
      .then((info) => {
        if (info && (info.departure || info.arrival)) {
          setFlightBriefing({ departure: info.departure, arrival: info.arrival });
        } else {
          return FlightService.GetBooking().then((b: any) => {
            if (b) {
              const dep = b.departure_airport?.icao;
              const arr = b.alternate_airport?.icao ?? b.arrival_airport?.icao;
              if (dep || arr) setFlightBriefing({ departure: dep, arrival: arr });
            }
          });
        }
      })
      .catch(() => {});
  }, [localMode, ready]);

  // Load document library
  useEffect(() => {
    let cancelled = false;
    setLoading(!localMode);
    setError("");
    if (localMode || !ready) return;

    DocumentService.GetDocuments(1, "", "")
      .then(async (result) => {
        if (cancelled) return;
        if (!result || result.status !== "ok") {
          setError(statusKey(result?.status ?? ""));
          return;
        }
        let items = (result.data ?? []) as DocItem[];
        const lastPage = result.last_page ?? 1;
        if (lastPage > 1) {
          const pagePromises: Promise<any>[] = [];
          for (let p = 2; p <= lastPage; p++) {
            pagePromises.push(DocumentService.GetDocuments(p, "", ""));
          }
          const nextPages = await Promise.all(pagePromises);
          if (cancelled) return;
          for (const pageResult of nextPages) {
            if (pageResult?.status === "ok" && Array.isArray(pageResult.data)) {
              items = items.concat(pageResult.data as DocItem[]);
            }
          }
        }
        setDocuments(items);

        // Select first non-folder document by default if nothing selected
        if (!selectedId && items.length > 0) {
          const firstReadable = items.find((d) => d.type !== "folder") ?? items[0];
          setSelectedId(firstReadable.id);
        }
      })
      .catch(() => {
        if (!cancelled) setError("documents.loadError");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [refresh, localMode, ready]);

  // Fetch full detail when a document is selected
  useEffect(() => {
    let cancelled = false;
    setDetailError("");
    setDetailDoc(null);
    if (!selectedId || localMode || !ready) return;

    // Fast preview from list cache while loading fresh detail
    const cached = documents.find((d) => d.id === selectedId);
    if (cached) setDetailDoc(cached);

    setDetailLoading(true);
    DocumentService.GetDocument(selectedId)
      .then((result) => {
        if (cancelled) return;
        if (!result || result.status !== "ok" || !result.data) {
          if (!cached) setDetailError(statusKey(result?.status ?? ""));
          return;
        }
        setDetailDoc(result.data as DocItem);
      })
      .catch(() => {
        if (!cancelled && !cached) setDetailError("documents.loadError");
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [selectedId, documents, localMode, ready]);

  // Categorize documents
  const categories = useMemo(() => {
    return [
      { id: "all", label: t("documents.catAll", "Todos"), icon: Layers },
      { id: "briefing", label: t("documents.catBriefings", "Airport Briefings"), icon: Plane },
      { id: "fdm", label: t("documents.catFDM", "FDM Profiles"), icon: ShieldAlert },
      { id: "manuals", label: t("documents.catManuals", "Manuais & SOPs"), icon: BookOpen },
      { id: "load", label: t("documents.catLoadProfiles", "Perfis de Carga"), icon: FileText },
    ];
  }, [t]);

  // Filtered documents
  const filteredDocuments = useMemo(() => {
    return documents.filter((doc) => {
      const titleLower = doc.title.toLowerCase();
      const searchLower = search.toLowerCase().trim();

      // Category matching. source_type is the server's stable key for
      // generated documents; the title and label heuristics cover documents
      // that were uploaded by hand.
      if (selectedCategory === "briefing") {
        const isBriefing =
          doc.source_type === "airport" ||
          titleLower.includes("briefing") ||
          titleLower.includes("airport") ||
          doc.source?.toLowerCase().includes("airport");
        if (!isBriefing) return false;
      } else if (selectedCategory === "fdm") {
        const isFDM =
          doc.source_type === "fdm_profile" ||
          titleLower.includes("fdm") ||
          titleLower.includes("safety") ||
          doc.source?.toLowerCase().includes("fdm");
        if (!isFDM) return false;
      } else if (selectedCategory === "load") {
        const isLoad =
          doc.source_type === "load_profile" ||
          titleLower.includes("load profile") ||
          doc.source?.toLowerCase().includes("load");
        if (!isLoad) return false;
      } else if (selectedCategory === "manuals") {
        const isManual =
          titleLower.includes("sop") ||
          titleLower.includes("manual") ||
          titleLower.includes("guide") ||
          titleLower.includes("procedimento") ||
          (!doc.is_auto_generated && doc.type !== "folder");
        if (!isManual) return false;
      }

      // Search matching
      if (searchLower) {
        return (
          titleLower.includes(searchLower) ||
          (doc.source && doc.source.toLowerCase().includes(searchLower))
        );
      }
      return true;
    });
  }, [documents, selectedCategory, search]);

  useEffect(() => {
    if (filteredDocuments.length > 0 && !filteredDocuments.some((d) => d.id === selectedId)) {
      setSelectedId(filteredDocuments[0].id);
    } else if (filteredDocuments.length === 0) {
      setSelectedId(null);
    }
  }, [filteredDocuments, selectedId]);

  const activeDoc = detailDoc ?? documents.find((d) => d.id === selectedId);

  const getDocTypeIcon = (doc: DocItem) => {
    if (doc.type === "folder") return <Folder className="h-4 w-4 text-amber-500" />;
    if (doc.type === "pdf" || doc.file_url) return <FileText className="h-4 w-4 text-red-400" />;
    if (doc.is_auto_generated) {
      if (doc.title.toLowerCase().includes("briefing")) {
        return <Plane className="h-4 w-4 text-sky-400" />;
      }
      return <ShieldAlert className="h-4 w-4 text-emerald-400" />;
    }
    return <BookOpen className="h-4 w-4 text-primary" />;
  };

  const getDocTypeBadge = (doc: DocItem) => {
    if (doc.is_auto_generated) {
      return (
        <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-emerald-500/40 text-emerald-400 bg-emerald-500/10">
          {t("documents.badgeAuto", "Auto")}
        </Badge>
      );
    }
    if (doc.type === "pdf" || doc.file_url) {
      return (
        <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-red-500/40 text-red-400 bg-red-500/10">
          {t("documents.badgePdf", "PDF")}
        </Badge>
      );
    }
    if (doc.type === "folder") {
      return (
        <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-amber-500/40 text-amber-400 bg-amber-500/10">
          {t("documents.badgeFolder", "Folder")}
        </Badge>
      );
    }
    return (
      <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-border text-muted-foreground">
        {t("documents.badgeHtml", "HTML")}
      </Badge>
    );
  };

  const formatDate = (val?: string) => {
    if (!val) return "";
    const d = new Date(val);
    return Number.isNaN(d.getTime())
      ? ""
      : d.toLocaleDateString(i18n.language, { day: "numeric", month: "short", year: "numeric" });
  };

  // PDFs render in place. The upstream route is Bearer-authenticated, so the
  // Go side proxies it at /documents/{id}/pdf on the app's own asset server;
  // the bytes are loaded into a blob URL and shown in an embedded viewer.
  const isPdfDoc = !!activeDoc && (activeDoc.type === "pdf" || (!!activeDoc.file_url && !activeDoc.content));
  const pdfDocId = isPdfDoc ? activeDoc!.id : null;
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);
  const [pdfError, setPdfError] = useState<string | null>(null);
  const [pdfLoading, setPdfLoading] = useState(false);

  useEffect(() => {
    if (!pdfDocId) {
      setPdfUrl(null);
      setPdfError(null);
      setPdfLoading(false);
      return;
    }
    let cancelled = false;
    let objectUrl: string | null = null;
    setPdfLoading(true);
    setPdfError(null);
    setPdfUrl(null);

    fetch(`/documents/${encodeURIComponent(pdfDocId)}/pdf`, { method: "GET", cache: "no-store" })
      .then(async (res) => {
        if (cancelled) return;
        if (!res.ok) {
          setPdfError(
            res.status === 403
              ? "documents.pdfAccessDenied"
              : res.status === 404
                ? "documents.pdfUnavailable"
                : "documents.pdfError"
          );
          return;
        }
        const blob = await res.blob();
        if (cancelled) return;
        objectUrl = URL.createObjectURL(blob);
        setPdfUrl(objectUrl);
      })
      .catch((err) => {
        if (cancelled) return;
        console.warn("Failed to load document PDF:", err);
        setPdfError("documents.pdfError");
      })
      .finally(() => {
        if (!cancelled) setPdfLoading(false);
      });

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [pdfDocId]);

  // Safe HTML content. The container classes match the airspace document
  // library (resources/views/livewire/documents/library.blade.php) so a
  // document reads the same here as on the web.
  const sanitizedHtml = useMemo(
    () => (activeDoc?.content ? sanitizeDocumentHtml(activeDoc.content) : ""),
    [activeDoc?.content]
  );

  const handleContentClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement | null;
    const anchor = target?.closest?.("a[href]") as HTMLAnchorElement | null;
    if (!anchor) return;
    const href = anchor.getAttribute("href");
    if (!href) return;
    if (href.startsWith("#")) return;
    e.preventDefault();
    void DocumentService.OpenDocumentURL(href);
  };

  return (
    <div className="flex h-full flex-col space-y-4" aria-label={t("documents.title", "Documentação")}>
      {/* Header section with title and search */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-semibold tracking-tight text-foreground">
              {t("documents.title", "Documentação")}
            </h2>
            {company && (
              <Badge variant="outline" className="text-xs border-border bg-card/60">
                {company}
              </Badge>
            )}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("documents.subtitle", "Biblioteca de Manuais, SOPs, FDM Profiles e Airport Briefings da VA")}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative w-48 sm:w-64">
            <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              type="text"
              placeholder={t("documents.searchPlaceholder", "Buscar documento...")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 pl-8 text-xs bg-card/60 border-border/80"
            />
            {search && (
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setSearch("")}
                className="absolute right-1 top-1.5 h-5 w-5 text-muted-foreground hover:text-foreground"
              >
                <X className="h-3 w-3" />
              </Button>
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((r) => r + 1)}
            disabled={loading || localMode}
            className="h-8 gap-1.5 text-xs border-border/80"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            <span className="hidden sm:inline">{t("documents.refresh", "Atualizar")}</span>
          </Button>
        </div>
      </div>

      {/* Flight Context Briefing Banner (when active flight/booking exists) */}
      {flightBriefing && (flightBriefing.departure || flightBriefing.arrival) && (
        <Card className="gap-0 border-primary/20 bg-primary/5 p-3 shadow-xs">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/15 text-primary">
                <Plane className="h-4 w-4" />
              </div>
              <div>
                <p className="text-xs font-semibold text-foreground">
                  {t("documents.flightBriefingTitle", "Briefing Operacional do Voo")}
                </p>
                <p className="text-[11px] text-muted-foreground">
                  {flightBriefing.departure || "—"} ➔ {flightBriefing.arrival || "—"}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {flightBriefing.departure && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSearch(flightBriefing.departure!)}
                  className="h-7 text-xs border-primary/30 hover:bg-primary/10"
                >
                  <Plane className="mr-1 h-3 w-3" />
                  {flightBriefing.departure}
                </Button>
              )}
              {flightBriefing.arrival && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setSearch(flightBriefing.arrival!)}
                  className="h-7 text-xs border-primary/30 hover:bg-primary/10"
                >
                  <Plane className="mr-1 h-3 w-3 rotate-90" />
                  {flightBriefing.arrival}
                </Button>
              )}
            </div>
          </div>
        </Card>
      )}

      {/* Category Pills */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border/60 pb-2">
        {categories.map((cat) => {
          const Icon = cat.icon;
          const isActive = selectedCategory === cat.id;
          return (
            <Button
              key={cat.id}
              variant={isActive ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setSelectedCategory(cat.id)}
              className={`h-7 gap-1.5 px-3 text-xs font-medium transition-colors ${
                isActive ? "bg-accent font-semibold text-foreground shadow-xs" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
              {cat.label}
            </Button>
          );
        })}
      </div>

      {/* Main Content Area: Split layout (Document list on left, Document viewer on right) */}
      <div className="flex min-h-0 flex-1 gap-4">
        {/* Document List Sidebar */}
        <Card className="flex w-72 shrink-0 flex-col overflow-hidden border-border/80 bg-card p-0 shadow-xs sm:w-80">
          <div className="border-b border-border/60 px-3 py-2 text-xs font-semibold text-muted-foreground">
            {t("documents.listHeader", "Documentos ({{count}})", { count: filteredDocuments.length })}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2 space-y-1">
            {loading ? (
              <div className="flex h-32 flex-col items-center justify-center gap-2 text-muted-foreground">
                <Loader2 className="h-5 w-5 animate-spin" />
                <span className="text-xs">{t("documents.loading", "Carregando documentos...")}</span>
              </div>
            ) : error ? (
              <div className="p-4 text-center text-xs text-muted-foreground">
                <AlertCircle className="mx-auto mb-2 h-6 w-6 text-amber-500/80" />
                <p className="font-medium text-foreground">{t(error)}</p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setRefresh((r) => r + 1)}
                  className="mt-3 h-7 text-xs"
                >
                  {t("documents.retry", "Tentar novamente")}
                </Button>
              </div>
            ) : filteredDocuments.length === 0 ? (
              <div className="p-6 text-center text-xs text-muted-foreground">
                <BookOpen className="mx-auto mb-2 h-6 w-6 text-muted-foreground/40" />
                <p>{t("documents.noDocuments", "Nenhum documento encontrado.")}</p>
                {search && (
                  <Button
                    variant="link"
                    size="sm"
                    onClick={() => setSearch("")}
                    className="mt-1 h-auto p-0 text-xs text-primary"
                  >
                    {t("documents.clearSearch", "Limpar busca")}
                  </Button>
                )}
              </div>
            ) : (
              filteredDocuments.map((doc) => {
                const isSelected = selectedId === doc.id;
                return (
                  <button
                    key={doc.id}
                    type="button"
                    onClick={() => setSelectedId(doc.id)}
                    className={`group flex w-full items-start gap-2.5 rounded-lg p-2.5 text-left text-xs transition-colors ${
                      isSelected
                        ? "bg-accent text-accent-foreground font-medium shadow-xs"
                        : "hover:bg-muted/50 text-foreground"
                    }`}
                  >
                    <div className="mt-0.5 shrink-0">{getDocTypeIcon(doc)}</div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-1.5">
                        <span className="truncate font-semibold tracking-tight">{doc.title}</span>
                        {getDocTypeBadge(doc)}
                      </div>
                      <div className="mt-1 flex items-center gap-2 text-[10px] text-muted-foreground">
                        {doc.source && <span className="truncate font-medium">{doc.source}</span>}
                        {doc.created_at && <span>{formatDate(doc.created_at)}</span>}
                      </div>
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </Card>

        {/* Document Reader Pane */}
        <Card className="flex min-w-0 flex-1 flex-col overflow-hidden border-border/80 bg-card p-0 shadow-xs">
          {activeDoc ? (
            <div className="flex h-full flex-col">
              {/* Document Header */}
              <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 p-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h3 className="truncate text-base font-semibold tracking-tight text-foreground">
                      {activeDoc.title}
                    </h3>
                    {getDocTypeBadge(activeDoc)}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                    {activeDoc.source && (
                      <span className="flex items-center gap-1">
                        <Sparkles className="h-3 w-3 text-primary" />
                        {activeDoc.source}
                      </span>
                    )}
                    {activeDoc.created_at && (
                      <span className="flex items-center gap-1">
                        <Calendar className="h-3 w-3" />
                        {formatDate(activeDoc.created_at)}
                      </span>
                    )}
                  </div>
                </div>

                {/* External Actions (PDF, Download, etc.) */}
              </div>

              {/* Document Content Body */}
              <div className="min-h-0 flex-1 overflow-y-auto p-5">
                {detailLoading && !detailDoc ? (
                  <div className="flex h-48 items-center justify-center gap-2 text-muted-foreground">
                    <Loader2 className="h-5 w-5 animate-spin" />
                    <span className="text-xs">{t("documents.loadingContent", "Carregando conteúdo...")}</span>
                  </div>
                ) : detailError ? (
                  <div className="p-6 text-center text-xs text-muted-foreground">
                    <AlertCircle className="mx-auto mb-2 h-6 w-6 text-amber-500" />
                    <p>{t(detailError)}</p>
                  </div>
                ) : isPdfDoc ? (
                  pdfUrl ? (
                    <iframe
                      title={activeDoc.title}
                      src={pdfUrl}
                      className="h-full min-h-[60vh] w-full rounded-md border border-border/60 bg-white"
                    />
                  ) : (
                    <div className="flex h-full flex-col items-center justify-center p-8 text-center">
                      <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-red-500/10 text-red-500 border border-red-500/20">
                        {pdfLoading ? <Loader2 className="h-8 w-8 animate-spin" /> : <FileText className="h-8 w-8" />}
                      </div>
                      <h4 className="text-base font-semibold text-foreground">{activeDoc.title}</h4>
                      <p className="mt-1.5 max-w-sm text-xs leading-relaxed text-muted-foreground">
                        {pdfLoading
                          ? t("documents.pdfDownloading", "Carregando PDF…")
                          : pdfError
                            ? t(pdfError)
                            : t("documents.pdfUnavailable", "Este documento não possui PDF disponível.")}
                      </p>
                    </div>
                  )
                ) : activeDoc.type === "folder" ? (
                  <div className="space-y-4">
                    <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                      <FolderOpen className="h-5 w-5 text-amber-500" />
                      <span>{t("documents.folderContents", "Conteúdo da Pasta")}</span>
                    </div>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                      {documents
                        .filter((d) => d.parent_id === activeDoc.id)
                        .map((child) => (
                          <Card
                            key={child.id}
                            onClick={() => setSelectedId(child.id)}
                            className="flex cursor-pointer items-center justify-between p-3 transition-colors hover:bg-accent/60"
                          >
                            <div className="flex items-center gap-2 truncate">
                              {getDocTypeIcon(child)}
                              <span className="truncate text-xs font-medium">{child.title}</span>
                            </div>
                            <ChevronRight className="h-4 w-4 text-muted-foreground" />
                          </Card>
                        ))}
                    </div>
                  </div>
                ) : sanitizedHtml ? (
                  <div
                    className="document-content prose prose-sm dark:prose-invert max-w-none text-zinc-800 dark:text-zinc-200 [contain:paint]"
                    style={{ contain: "paint" }}
                    onClick={handleContentClick}
                    dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
                  />
                ) : (
                  <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
                    {t("documents.noContent", "Nenhum conteúdo disponível para este documento.")}
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="flex h-full flex-col items-center justify-center p-8 text-center text-muted-foreground">
              <BookOpen className="mb-2 h-10 w-10 text-muted-foreground/30" />
              <p className="text-sm font-medium">{t("documents.selectDocument", "Selecione um documento para ler")}</p>
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
