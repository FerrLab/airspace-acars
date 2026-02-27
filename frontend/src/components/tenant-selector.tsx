import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useAuth, type TenantInfo } from "@/context/auth-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Loader2, Search, Building2, CheckCircle2 } from "lucide-react";
import { AppLogo } from "@/components/app-logo";
import { AuthService } from "../../bindings/airspace-acars";

interface TenantSelectorProps {
  onTenantSelected: (hasToken: boolean) => void;
}

export function TenantSelector({ onTenantSelected }: TenantSelectorProps) {
  const { t } = useTranslation();
  const { setTenant, storedTokens, loginWithStoredToken } = useAuth();
  const [tenants, setTenants] = useState<TenantInfo[]>([]);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const result = await AuthService.FetchTenants();
        if (cancelled) return;
        setTenants(
          result.map((t) => ({
            id: t.id,
            name: t.name,
            domain: t.domains[0],
            logo_url: t.logo_url ?? undefined,
            banner_url: t.banner_url ?? undefined,
          }))
        );
        setStatus("ready");
      } catch {
        if (!cancelled) setStatus("error");
      }
    }

    load();
    return () => { cancelled = true; };
  }, []);

  const authenticatedTenants = tenants.filter((t) => storedTokens[t.id]);
  const unauthenticatedTenants = tenants.filter((t) => !storedTokens[t.id]);

  const filteredUnauthenticated = unauthenticatedTenants.filter(
    (tenant) =>
      tenant.name.toLowerCase().includes(search.toLowerCase()) ||
      tenant.domain.toLowerCase().includes(search.toLowerCase())
  );

  async function handleSelectAuthenticated(tenant: TenantInfo) {
    await AuthService.SelectTenant(tenant.domain);
    loginWithStoredToken(tenant);
    onTenantSelected(true);
  }

  async function handleSelectNew(tenant: TenantInfo) {
    await AuthService.SelectTenant(tenant.domain);
    setTenant(tenant);
    onTenantSelected(false);
  }

  return (
    <div className="flex h-full items-center justify-center bg-background">
      <Card className="w-[480px] border-border/50">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center">
            <AppLogo className="h-12 w-12" />
          </div>
          <CardTitle className="text-2xl tracking-tight">{t("tenant.title")}</CardTitle>
          <p className="text-sm text-muted-foreground">
            {t("tenant.subtitle")}
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {status === "loading" && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {status === "error" && (
            <div className="text-center py-8">
              <p className="text-sm text-destructive">
                {t("tenant.loadError")}
              </p>
            </div>
          )}

          {status === "ready" && (
            <>
              {authenticatedTenants.length > 0 && (
                <>
                  <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                    {t("tenant.loggedIn")}
                  </p>
                  <div className="space-y-1">
                    {authenticatedTenants.map((tenant) => (
                      <button
                        key={tenant.id}
                        onClick={() => handleSelectAuthenticated(tenant)}
                        className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-muted"
                      >
                        {tenant.logo_url ? (
                          <img
                            src={tenant.logo_url}
                            alt=""
                            className="h-8 w-8 rounded object-contain"
                          />
                        ) : (
                          <div className="flex h-8 w-8 items-center justify-center rounded bg-muted">
                            <Building2 className="h-4 w-4 text-muted-foreground" />
                          </div>
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{tenant.name}</p>
                          <p className="truncate text-xs text-muted-foreground">{tenant.domain}</p>
                        </div>
                        <CheckCircle2 className="h-4 w-4 shrink-0 text-green-500" />
                      </button>
                    ))}
                  </div>
                  <Separator />
                </>
              )}

              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                {authenticatedTenants.length > 0 ? t("tenant.otherOrgs") : t("tenant.organizations")}
              </p>
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t("tenant.search")}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="pl-9"
                />
              </div>
              <div className="max-h-[240px] overflow-y-auto space-y-1">
                {filteredUnauthenticated.map((tenant) => (
                  <button
                    key={tenant.id}
                    onClick={() => handleSelectNew(tenant)}
                    className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-muted"
                  >
                    {tenant.logo_url ? (
                      <img
                        src={tenant.logo_url}
                        alt=""
                        className="h-8 w-8 rounded object-contain"
                      />
                    ) : (
                      <div className="flex h-8 w-8 items-center justify-center rounded bg-muted">
                        <Building2 className="h-4 w-4 text-muted-foreground" />
                      </div>
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{tenant.name}</p>
                      <p className="truncate text-xs text-muted-foreground">{tenant.domain}</p>
                    </div>
                  </button>
                ))}
                {filteredUnauthenticated.length === 0 && (
                  <p className="py-4 text-center text-sm text-muted-foreground">
                    {t("tenant.noResults")}
                  </p>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
