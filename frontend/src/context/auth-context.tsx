import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { Events } from "@wailsio/runtime";
import { AuthService } from "../../bindings/airspace-acars";

export interface TenantInfo {
  id: string;
  name: string;
  domain: string;
  logo_url?: string;
  banner_url?: string;
}

interface StoredTokens {
  [tenantId: string]: { token: string; tenant: TenantInfo };
}

interface AuthContextType {
  isAuthenticated: boolean;
  // True once the Go backend has acknowledged the current token. Callers that
  // make authenticated API calls right after mount (the chat unread poll, in
  // particular) must wait for this: the backend only has `token` after an
  // async Wails round-trip, and firing a request before it lands means the
  // request goes out with no Authorization header and comes back 401.
  tokenSynced: boolean;
  token: string | null;
  tenant: TenantInfo | null;
  storedTokens: StoredTokens;
  setAuthenticated: (token: string) => void;
  setTenant: (tenant: TenantInfo) => void;
  loginWithStoredToken: (tenant: TenantInfo) => void;
  logout: () => void;
}

const TOKENS_KEY = "acars_tokens";
const TENANT_KEY = "acars_tenant";

function loadStoredTokens(): StoredTokens {
  try {
    // Migrate legacy single-token storage
    const legacyToken = localStorage.getItem("acars_token");
    const legacyTenant = localStorage.getItem(TENANT_KEY);
    if (legacyToken && legacyTenant) {
      const tenant = JSON.parse(legacyTenant) as TenantInfo;
      const migrated: StoredTokens = { [tenant.id]: { token: legacyToken, tenant } };
      localStorage.setItem(TOKENS_KEY, JSON.stringify(migrated));
      localStorage.removeItem("acars_token");
      return migrated;
    }
    localStorage.removeItem("acars_token");

    const raw = localStorage.getItem(TOKENS_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function saveStoredTokens(tokens: StoredTokens) {
  localStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
}

// Sync token to the Go backend so services can make authenticated API calls
function syncTokenToBackend(token: string | null): Promise<void> {
  return AuthService.SetToken(token ?? "").catch(() => {});
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [storedTokens, setStoredTokens] = useState<StoredTokens>(loadStoredTokens);
  const [token, setToken] = useState<string | null>(() => {
    const stored = localStorage.getItem(TENANT_KEY);
    if (!stored) return null;
    const tenant = JSON.parse(stored) as TenantInfo;
    const entry = loadStoredTokens()[tenant.id];
    return entry?.token ?? null;
  });
  const [tenant, setTenantState] = useState<TenantInfo | null>(() => {
    const stored = localStorage.getItem(TENANT_KEY);
    return stored ? JSON.parse(stored) : null;
  });

  const isAuthenticated = token !== null;

  // Starts false whenever there is a token to hand the backend, so a caller
  // gating on it (App.tsx) waits out the round-trip below instead of
  // assuming isAuthenticated already means the backend has it too.
  const [tokenSynced, setTokenSynced] = useState(token === null);

  // Sync token + tenant to backend on mount and whenever they change
  useEffect(() => {
    let cancelled = false;
    setTokenSynced(token === null);

    syncTokenToBackend(token).then(() => {
      if (!cancelled) setTokenSynced(true);
    });

    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    if (tenant) {
      AuthService.SelectTenant(tenant.domain).catch(() => {});
    }
  }, [tenant]);

  const setAuthenticated = useCallback((newToken: string) => {
    setToken(newToken);
    setTenantState((current) => {
      if (current) {
        setStoredTokens((prev) => {
          const next = { ...prev, [current.id]: { token: newToken, tenant: current } };
          saveStoredTokens(next);
          return next;
        });
      }
      return current;
    });
  }, []);

  const setTenant = useCallback((t: TenantInfo) => {
    localStorage.setItem(TENANT_KEY, JSON.stringify(t));
    setTenantState(t);
  }, []);

  const loginWithStoredToken = useCallback((t: TenantInfo) => {
    const entry = loadStoredTokens()[t.id];
    if (!entry) return;
    localStorage.setItem(TENANT_KEY, JSON.stringify(t));
    setTenantState(t);
    setToken(entry.token);
  }, []);

  const logout = useCallback(() => {
    setTenantState((current) => {
      if (current) {
        setStoredTokens((prev) => {
          const next = { ...prev };
          delete next[current.id];
          saveStoredTokens(next);
          return next;
        });
      }
      return null;
    });
    localStorage.removeItem(TENANT_KEY);
    setToken(null);
    syncTokenToBackend(null);
  }, []);

  // The backend fires this the moment a request comes back 401: the stored
  // token has been revoked or has expired. Without this the app has no way
  // to learn that and keeps polling with the same dead token forever, so the
  // pilot never notices they need to sign in again.
  useEffect(() => {
    return Events.On("session-expired", () => {
      logout();
    });
  }, [logout]);

  return (
    <AuthContext.Provider value={{ isAuthenticated, tokenSynced, token, tenant, storedTokens, setAuthenticated, setTenant, loginWithStoredToken, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
