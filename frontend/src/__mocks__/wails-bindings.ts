// Factory functions for mocking Wails binding services in tests.

export function mockFlightLogService() {
  return {
    GetMyFlights: (_page: number, _limit: number) =>
      Promise.resolve({
        status: "ok",
        pilot: {
          pilot_id: "1",
          name: "Captain John Doe",
          callsign: "GLO101",
          rank: "Captain",
          rank_image_url: "",
          total_flights: 12,
          total_hours: 48.5,
          avg_landing_rate: -160,
          total_distance_nm: 6420,
          points: 350,
        },
        flights: [] as Array<any>,
        current_page: 1,
        page: 1,
        last_page: 1,
        total: 0,
      }),
  };
}

export function mockNOTAMService() {
  return {
    GetNOTAMs: (_page: number) =>
      Promise.resolve({
        status: "ok",
        data: [] as Array<{ id: string; title: string; content: string; created_at: string }>,
        current_page: 1,
        last_page: 1,
      }),
    GetNOTAM: (_id: string) =>
      Promise.resolve({
        status: "ok",
        data: null as { id: string; title: string; content: string; created_at: string } | null,
      }),
  };
}

export function mockDocumentService() {
  return {
    GetDocuments: (_page: number, _parentID: string, _search: string) =>
      Promise.resolve({
        status: "ok",
        data: [] as Array<{
          id: string;
          title: string;
          type: string;
          parent_id?: string;
          visibility?: string;
          is_auto_generated: boolean;
          source?: string;
          content?: string;
          file_url?: string;
          sort_order: number;
          created_at?: string;
          updated_at?: string;
        }>,
        current_page: 1,
        last_page: 1,
      }),
    GetDocument: (_id: string) =>
      Promise.resolve({
        status: "ok",
        data: null as {
          id: string;
          title: string;
          type: string;
          parent_id?: string;
          visibility?: string;
          is_auto_generated: boolean;
          source?: string;
          content?: string;
          file_url?: string;
          sort_order: number;
          created_at?: string;
          updated_at?: string;
        } | null,
      }),
    OpenDocumentURL: (_url: string) => Promise.resolve(),
  };
}
export function mockSettingsService() {
  return {
    GetSettings: () =>
      Promise.resolve({
        theme: "dark",
        simType: "auto",
        xplaneHost: "127.0.0.1",
        xplanePort: 49000,
        xplanePath: "",
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
    GetActiveFlightInfo: () =>
      Promise.resolve(null as { departure?: string; arrival?: string; callsign?: string } | null),
    GetBooking: () =>
      Promise.resolve({
        id: "bk_test_1",
        callsign: "BAW123",
        departure_airport: { icao: "EGLL", city: "London" },
        arrival_airport: { icao: "KJFK", city: "New York" },
      }),
    StartFlight: (
      _callsign: string,
      _departure: string,
      _arrival: string,
      _bookingID: string,
    ) => Promise.resolve(),
    StopFlight: () => Promise.resolve(),
    FinishFlight: () => Promise.resolve(),
    CancelFinish: () => Promise.resolve(),
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

export function mockFlightDataService() {
  return {
    ConnectSim: (_simType: string) => Promise.resolve(""),
    DisconnectSim: () => {},
    IsConnected: () => false,
    ConnectedAdapter: () => "",
    GetFlightDataNow: () => Promise.resolve(null),
    StartRecording: () => Promise.resolve(),
    StopRecording: () => {},
    IsRecording: () => Promise.resolve(false),
    GetRecordingInfo: () => Promise.resolve({}),
    ExportCSV: (_filePath: string) => Promise.resolve(),
  };
}

export function mockAudioService() {
  return {
    FetchSoundInstructions: () => Promise.resolve([]),
    GetAudioData: (_filename: string) => Promise.resolve(null),
  };
}

export function mockDiscordService() {
  return {
    SetEnabled: (_enabled: boolean) => Promise.resolve(),
  };
}

export function mockUpdateService() {
  return {
    CheckForUpdate: () => Promise.resolve(),
    ApplyUpdate: () => Promise.resolve(),
    GetCurrentVersion: () => Promise.resolve(""),
    TailLogs: () => Promise.resolve(""),
  };
}

export function mockDebugService() {
  return {
    GetDebugSnapshot: () =>
      Promise.resolve({
        ground: { airport: "", runways: 0, stands: 0, runway: "", stand: "", loadedAt: "0001-01-01T00:00:00Z", checkedAt: "0001-01-01T00:00:00Z", lastError: "" },
        report: { json: "", at: "0001-01-01T00:00:00Z", outcome: "", batchSize: 0 },
        profiles: [],
      }),
  };
}
