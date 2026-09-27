import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, FileText, Loader2, RefreshCw } from "lucide-react";
import { NOTAMService } from "../../bindings/airspace-acars";
import { useAuth } from "@/context/auth-context";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { notamText } from "@/lib/notam-text";

interface Notice {
  id: string;
  title: string;
  content: string;
  created_at: string;
}

const knownStatuses = ["accessDenied", "unavailable", "rateLimited", "noSession", "localMode"];
function statusKey(status: string) {
  return `notams.${knownStatuses.includes(status) ? status : "loadError"}`;
}

export function NotamsTab({ localMode = false }: { localMode?: boolean }) {
  const { tenant, tokenSynced } = useAuth();
  // No cached notices survive a company change, including in-flight replies.
  return <CompanyNotams key={`${tenant?.id}:${tenant?.domain}`} company={tenant?.name ?? ""} localMode={localMode} ready={tokenSynced && !!tenant} />;
}

function CompanyNotams({ company, localMode, ready }: { company: string; localMode: boolean; ready: boolean }) {
  const { t, i18n } = useTranslation();
  const [page, setPage] = useState(1);
  const [lastPage, setLastPage] = useState(1);
  const [refresh, setRefresh] = useState(0);
  const [notices, setNotices] = useState<Notice[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [detail, setDetail] = useState<Notice | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setNotices([]);
    setSelected(null);
    setError("");
    setLoading(!localMode);
    if (localMode || !ready) return;
    NOTAMService.GetNOTAMs(page).then((result) => {
      if (cancelled) return;
      if (!result || result.status !== "ok") {
        setError(statusKey(result?.status ?? ""));
        return;
      }
      const items = result.data ?? [];
      setNotices(items);
      setLastPage(Math.max(1, result.last_page));
      setSelected(items[0]?.id ?? null);
    }).catch(() => {
      if (!cancelled) setError("notams.loadError");
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [page, refresh, localMode, ready]);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setDetailError("");
    setDetailLoading(false);
    if (!selected || localMode || !ready) return;
    setDetailLoading(true);
    NOTAMService.GetNOTAM(selected).then((result) => {
      if (cancelled) return;
      if (!result || result.status !== "ok" || !result.data) {
        setDetailError(statusKey(result?.status ?? ""));
        return;
      }
      setDetail(result.data);
    }).catch(() => {
      if (!cancelled) setDetailError("notams.loadError");
    }).finally(() => {
      if (!cancelled) setDetailLoading(false);
    });
    return () => { cancelled = true; };
  }, [selected, localMode, ready]);

  function dateLabel(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString(i18n.language, { day: "numeric", month: "short", year: "numeric" });
  }

  const body = detail ? notamText(detail.content) : "";
  const title = (notice: Notice) => notice.title.trim() || t("notams.untitled");

  return (
    <section className="space-y-6" aria-labelledby="notams-heading">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="notams-heading" className="text-lg font-semibold tracking-tight">{t("notams.title")}</h2>
          <p className="text-sm text-muted-foreground">{t("notams.subtitle", { company })}</p>
        </div>
        <Button variant="outline" size="sm" disabled={loading || localMode || !ready} onClick={() => { setPage(1); setRefresh((value) => value + 1); }}>
          <RefreshCw className="mr-2 h-3.5 w-3.5" aria-hidden="true" />{t("notams.refresh")}
        </Button>
      </div>
      <Separator />
      {localMode ? (
        <p className="py-12 text-center text-sm text-muted-foreground">{t("notams.localMode")}</p>
      ) : loading ? (
        <div role="status" className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />{t("notams.loading")}
        </div>
      ) : error ? (
        <div role="alert" className="rounded-lg border border-border bg-muted/30 p-6 text-sm">{t(error)}</div>
      ) : (
        <>
          {notices.length === 0 ? (
            <div role="status" className="space-y-2 py-12 text-center">
              <FileText className="mx-auto mb-3 h-8 w-8 text-muted-foreground" aria-hidden="true" />
              <p className="text-sm font-medium">{t("notams.empty")}</p>
              <p className="text-sm text-muted-foreground">{t("notams.emptyDescription")}</p>
            </div>
          ) : (
            <div className="grid items-start gap-4 lg:grid-cols-[minmax(220px,1fr)_minmax(0,2fr)]">
              <nav aria-label={t("notams.listLabel")} className="overflow-hidden rounded-lg border border-border">
                {notices.map((notice) => (
                  <button key={notice.id} type="button" aria-current={selected === notice.id ? "true" : undefined} onClick={() => setSelected(notice.id)}
                    className={`block w-full border-b border-border px-4 py-3 text-left last:border-b-0 focus-visible:outline-2 focus-visible:outline-ring focus-visible:-outline-offset-2 ${selected === notice.id ? "bg-accent text-accent-foreground" : "hover:bg-muted/50"}`}>
                    <span className="block break-words text-sm font-medium">{title(notice)}</span>
                    {dateLabel(notice.created_at) && <span className="mt-1 block text-xs text-muted-foreground">{dateLabel(notice.created_at)}</span>}
                  </button>
                ))}
              </nav>
              <article aria-live="polite" aria-busy={detailLoading} className="min-w-0 rounded-lg border border-border bg-card p-5">
                {detailLoading ? <p role="status" className="text-sm text-muted-foreground">{t("notams.loading")}</p>
                  : detailError ? <p role="alert" className="text-sm">{t(detailError)}</p>
                  : detail && <>
                    <h3 className="break-words text-lg font-semibold">{title(detail)}</h3>
                    {dateLabel(detail.created_at) && <p className="mt-1 text-xs text-muted-foreground">{t("notams.published", { date: dateLabel(detail.created_at) })}</p>}
                    <Separator className="my-4" />
                    <div className="whitespace-pre-wrap break-words text-sm leading-7 [overflow-wrap:anywhere]">{body || t("notams.noContent")}</div>
                  </>}
              </article>
            </div>
          )}
          {lastPage > 1 && <div className="flex items-center justify-between gap-3">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}><ChevronLeft className="mr-1 h-4 w-4" aria-hidden="true" />{t("notams.previous")}</Button>
            <span className="text-xs text-muted-foreground">{t("notams.page", { page, total: lastPage })}</span>
            <Button variant="outline" size="sm" disabled={page >= lastPage} onClick={() => setPage((value) => value + 1)}>{t("notams.next")}<ChevronRight className="ml-1 h-4 w-4" aria-hidden="true" /></Button>
          </div>}
        </>
      )}
    </section>
  );
}
