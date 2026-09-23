import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { AuthProvider, useAuth } from "./auth-context";

const setToken = vi.fn(() => Promise.resolve());

vi.mock("../../bindings/airspace-acars", () => ({
  AuthService: {
    SetToken: (token: string) => setToken(token),
    SelectTenant: () => Promise.resolve(),
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
  expect(setToken).toHaveBeenCalledWith("stored-token");

  resolveSetToken();

  await waitFor(() => {
    expect(readState()).toEqual({ isAuthenticated: true, tokenSynced: true });
  });
});
