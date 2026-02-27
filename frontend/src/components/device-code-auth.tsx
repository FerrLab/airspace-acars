import { useState, useEffect, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/context/auth-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Loader2, Radio, ArrowLeft, ExternalLink, AlertTriangle } from "lucide-react";
import { AuthService } from "../../bindings/airspace-acars";

interface DeviceCodeAuthProps {
  onBack: () => void;
}

export function DeviceCodeAuth({ onBack }: DeviceCodeAuthProps) {
  const { t } = useTranslation();
  const { tenant, setAuthenticated } = useAuth();
  const [userCode, setUserCode] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "polling" | "success" | "expired" | "error">("idle");
  const authTokenRef = useRef<string | null>(null);
  const intervalRef = useRef<number>(5000);

  const startAuth = useCallback(async () => {
    setStatus("idle");
    setUserCode(null);
    authTokenRef.current = null;
    intervalRef.current = 5000;

    try {
      const resp = await AuthService.RequestDeviceCode();
      if (!resp) throw new Error("no response");
      setUserCode(resp.user_code);
      authTokenRef.current = resp.authorization_token;
      setStatus("polling");
    } catch {
      setStatus("error");
    }
  }, []);

  useEffect(() => {
    startAuth();
  }, [startAuth]);

  useEffect(() => {
    if (status !== "polling" || !authTokenRef.current) return;
    let cancelled = false;
    let timeoutId: ReturnType<typeof setTimeout>;

    async function poll() {
      if (cancelled || !authTokenRef.current) return;

      try {
        const resp = await AuthService.PollForToken(authTokenRef.current);
        if (cancelled || !resp) return;

        switch (resp.status) {
          case 200:
            setStatus("success");
            setTimeout(() => setAuthenticated(resp.access_token!), 500);
            return;
          case 202:
            break;
          case 410:
            setStatus("expired");
            return;
          case 429:
            intervalRef.current = Math.min(intervalRef.current * 2, 30000);
            break;
        }
      } catch {
        // network error — keep polling
      }

      timeoutId = setTimeout(poll, intervalRef.current);
    }

    timeoutId = setTimeout(poll, intervalRef.current);

    return () => {
      cancelled = true;
      clearTimeout(timeoutId);
    };
  }, [status, setAuthenticated]);

  async function handleOpenAuth() {
    if (!userCode) return;
    try {
      await AuthService.OpenAuthorizationURL(userCode);
    } catch {
      // fallback: user can manually navigate
    }
  }

  return (
    <div className="flex h-full items-center justify-center bg-background">
      <Card className="w-[420px] border-border/50">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Radio className="h-6 w-6 text-primary" />
          </div>
          <CardTitle className="text-2xl tracking-tight">{t("auth.title")}</CardTitle>
          <p className="text-sm text-muted-foreground">
            {t("auth.signingIn", { org: tenant?.name ?? "your organization" })}
          </p>
        </CardHeader>
        <CardContent className="space-y-6">
          {status === "idle" && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {(status === "polling" || status === "success") && userCode && (
            <>
              <div className="text-center">
                <p className="mb-3 text-sm text-muted-foreground">
                  {t("auth.enterCode")}
                </p>
                <div className="inline-flex items-center gap-2 rounded-lg border border-border bg-muted px-6 py-3">
                  <span className="font-mono text-3xl font-bold tracking-[0.3em] tabular-nums">
                    {userCode}
                  </span>
                </div>
              </div>

              <div className="flex justify-center">
                <Button variant="outline" onClick={handleOpenAuth}>
                  <ExternalLink className="mr-2 h-4 w-4" />
                  {t("auth.openAuth")}
                </Button>
              </div>

              <div className="flex items-center justify-center gap-2">
                {status === "polling" ? (
                  <>
                    <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                    <span className="text-xs text-muted-foreground">
                      {t("auth.waiting")}
                    </span>
                  </>
                ) : (
                  <Badge variant="default" className="bg-green-600">
                    {t("auth.authorized")}
                  </Badge>
                )}
              </div>
            </>
          )}

          {status === "expired" && (
            <div className="space-y-4 text-center py-4">
              <div className="flex items-center justify-center gap-2 text-amber-500">
                <AlertTriangle className="h-5 w-5" />
                <p className="text-sm font-medium">{t("auth.expired")}</p>
              </div>
              <Button variant="outline" onClick={startAuth}>
                {t("auth.tryAgain")}
              </Button>
            </div>
          )}

          {status === "error" && (
            <div className="text-center py-8">
              <p className="text-sm text-destructive">
                {t("auth.failed")}
              </p>
              <Button variant="outline" className="mt-4" onClick={startAuth}>
                {t("auth.retry")}
              </Button>
            </div>
          )}

          <div className="flex justify-center">
            <Button variant="ghost" size="sm" onClick={onBack}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t("auth.changeOrg")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
