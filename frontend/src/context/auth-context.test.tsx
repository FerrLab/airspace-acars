import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { AuthProvider, useAuth } from "./auth-context";

const setToken = vi.fn(() => Promise.resolve());
const selectTenant = vi.fn(() => Promise.resolve());

// Every call either service receives, in order, so a test can assert that the
// backend is told which tenant it is talking to before it is given a token.
const calls: string[] = [];

vi.mock("../../bindings/airspace-acars", () => ({
  AuthService: {
    SetToken: (token: string) => {
      calls.push("SetToken");
      return setToken(token);
    },
    SelectTenant: (domain: string) => {
      calls.push("SelectTenant");
      return selectTenant(domain);
    },
  },
}));

const TENANT = { id: "tenant-1", name: "Test Tenant", domain: "test.airspace.ferrlab.com" };

function Probe() {
  const { isAuthenticated, tokenSynced } = useAuth();
  return <div data-testid="state">{JSON.stringify({ isAuthenticated, tokenSynced })}</div>;
}

function readState() {
  return JSON.parse(screen.getByTestId("state").textContent!);
}

beforeEach(() => {
  localStorage.clear();
  setToken.mockClear();
  selectTenant.mockClear();
  calls.length = 0;
});

it("stays synced immediately when there is no stored session", () => {
  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>,
  );

  expect(readState()).toEqual({ isAuthenticated: false, tokenSynced: true });
});

// The regression this covers (AIRSPACE-ACARS-A): a stored token makes
// isAuthenticated true synchronously on mount, but the Go backend only has it
// after AuthService.SetToken's async round-trip resolves. A caller that reads
// isAuthenticated alone and fires an authenticated request immediately would
// race the backend and get a 401.
it("is not synced until the stored token round-trip to the backend resolves", async () => {
  let resolveSetToken!: () => void;
  setToken.mockReturnValueOnce(new Promise<void>((resolve) => (resolveSetToken = resolve)));

  localStorage.setItem("acars_tenant", JSON.stringify(TENANT));
  localStorage.setItem("acars_tokens", JSON.stringify({ [TENANT.id]: { token: "stored-token", tenant: TENANT } }));

  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>,
  );

  // isAuthenticated flips true synchronously (the token is already known
  // locally); tokenSynced must not follow until the backend confirms it.
  expect(readState()).toEqual({ isAuthenticated: true, tokenSynced: false });

  await waitFor(() => expect(setToken).toHaveBeenCalledWith("stored-token"));
  expect(readState()).toEqual({ isAuthenticated: true, tokenSynced: false });

  resolveSetToken();

  await waitFor(() => {
    expect(readState()).toEqual({ isAuthenticated: true, tokenSynced: true });
  });
});

// A token is only valid against the tenant that issued it. These were two
// effects with no ordering, so the backend could be handed a new token while
// still pointing at the previous tenant's URL — rejected with a 401, which
// signed the pilot out of the tenant they had just signed in to.
it("tells the backend which tenant it is talking to before handing it a token", async () => {
  localStorage.setItem("acars_tenant", JSON.stringify(TENANT));
  localStorage.setItem("acars_tokens", JSON.stringify({ [TENANT.id]: { token: "stored-token", tenant: TENANT } }));

  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>,
  );

  await waitFor(() => expect(readState().tokenSynced).toBe(true));

  expect(selectTenant).toHaveBeenCalledWith(TENANT.domain);
  expect(calls).toEqual(["SelectTenant", "SetToken"]);
});

// The shell must not mount on a half-applied identity: while the tenant is
// being set, the backend still holds the previous one, and the token about to
// be sent does not belong to it.
it("stays unsynced until both the tenant and the token have landed", async () => {
  let resolveSelectTenant!: () => void;
  selectTenant.mockReturnValueOnce(new Promise<void>((resolve) => (resolveSelectTenant = resolve)));

  localStorage.setItem("acars_tenant", JSON.stringify(TENANT));
  localStorage.setItem("acars_tokens", JSON.stringify({ [TENANT.id]: { token: "stored-token", tenant: TENANT } }));

  render(
    <AuthProvider>
      <Probe />
    </AuthProvider>,
  );

  // Tenant selection is still in flight, so the token must not have gone yet.
  await waitFor(() => expect(selectTenant).toHaveBeenCalled());
  expect(setToken).not.toHaveBeenCalled();
  expect(readState().tokenSynced).toBe(false);

  resolveSelectTenant();

  await waitFor(() => {
    expect(readState()).toEqual({ isAuthenticated: true, tokenSynced: true });
  });
  expect(calls).toEqual(["SelectTenant", "SetToken"]);
});
