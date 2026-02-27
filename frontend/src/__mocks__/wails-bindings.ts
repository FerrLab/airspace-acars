// Factory functions for mocking Wails binding services in tests.

export function mockSettingsService() {
  return {
    GetSettings: () =>
      Promise.resolve({
        theme: "dark",
        simType: "auto",
        xplaneHost: "127.0.0.1",
        xplanePort: 49000,
        apiBaseURL: "https://airspace.ferrlab.com",
        localMode: false,
        chatSound: "default",
        discordPresence: true,
      }),
    UpdateSettings: (_settings: unknown) => Promise.resolve(),
  };
}

export function mockFlightService() {
  return {
    GetFlightState: () => Promise.resolve("idle"),
    GetBooking: () =>
      Promise.resolve({ callsign: "BAW123", departure: "EGLL", arrival: "KJFK" }),
    StartFlight: () => Promise.resolve(),
    StopFlight: () => Promise.resolve(),
    FinishFlight: () => Promise.resolve(),
  };
}

export function mockAuthService() {
  return {
    FetchTenants: () => Promise.resolve([]),
    SelectTenant: (_domain: string) => Promise.resolve(),
    RequestDeviceCode: () =>
      Promise.resolve({ user_code: "ABCD-1234", authorization_token: "tok" }),
    PollForToken: (_token: string) =>
      Promise.resolve({ access_token: "", status: 202, error: "" }),
    SetToken: (_token: string) => Promise.resolve(),
  };
}

export function mockChatService() {
  return {
    GetMessages: (_page: number) =>
      Promise.resolve({ data: [], current_page: 1, last_page: 1 }),
    SendMessage: (_message: string) =>
      Promise.resolve({ id: 1, message: _message }),
    ConfirmMessage: (_id: number) => Promise.resolve(),
  };
}
