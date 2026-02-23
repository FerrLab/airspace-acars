import { useState, useEffect, useRef } from "react";
import { useAuth } from "@/context/auth-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Loader2, Radio } from "lucide-react";
import { AuthService } from "../../bindings/airspace-acars";

export function LoginScreen() {
  const { setAuthenticated } = useAuth();
  const [userCode, setUserCode] = useState<string | null>(null);
  const [verificationUri, setVerificationUri] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "polling" | "success" | "error">("idle");
  const deviceCodeRef = useRef<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function startAuth() {
      try {
        const resp = await AuthService.RequestDeviceCode();
        if (cancelled) return;
        setUserCode(resp.user_code);
        setVerificationUri(resp.verification_uri);
        deviceCodeRef.current = resp.device_code;
        setStatus("polling");
      } catch {
        if (!cancelled) setStatus("error");
      }
    }

    startAuth();
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (status !== "polling" || !deviceCodeRef.current) return;
    let cancelled = false;

    const interval = setInterval(async () => {
      try {
        const resp = await AuthService.PollForToken(deviceCodeRef.current!);
        if (cancelled) return;
        if (resp.access_token) {
          setStatus("success");
          clearInterval(interval);
          setTimeout(() => setAuthenticated(resp.access_token), 500);
        }
      } catch {
        // keep polling
      }
    }, 5000);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [status, setAuthenticated]);

  return (
    <div className="flex h-full items-center justify-center bg-background">
      <Card className="w-[420px] border-border/50">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Radio className="h-6 w-6 text-primary" />
          </div>
          <CardTitle className="text-2xl tracking-tight">Airspace ACARS</CardTitle>
          <p className="text-sm text-muted-foreground">
            Sign in to your account
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
                  Enter this code at the activation page
                </p>
                <div className="inline-flex items-center gap-2 rounded-lg border border-border bg-muted px-6 py-3">
                  <span className="font-mono text-3xl font-bold tracking-[0.3em] tabular-nums">
                    {userCode}
                  </span>
                </div>
              </div>

              {verificationUri && (
                <div className="text-center">
                  <p className="text-xs text-muted-foreground">Activation URL</p>
                  <a
                    href={verificationUri}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm text-primary underline-offset-4 hover:underline"
                  >
                    {verificationUri}
                  </a>
                </div>
              )}

              <div className="flex items-center justify-center gap-2">
                {status === "polling" ? (
                  <>
                    <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                    <span className="text-xs text-muted-foreground">
                      Waiting for authorization...
                    </span>
                  </>
                ) : (
                  <Badge variant="default" className="bg-green-600">
                    Authorized
                  </Badge>
                )}
              </div>
            </>
          )}

          {status === "error" && (
            <div className="text-center py-8">
              <p className="text-sm text-destructive">
                Failed to start authentication. Please restart the app.
              </p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
