// Mock for the Wails-generated `bindings/airspace-acars` module.
//
// That module is produced by `wails3 generate bindings` and is gitignored
// (frontend/bindings), so it does not exist in a plain checkout — including
// CI. Anything that imports it, even transitively through a component a test
// never renders, fails at Vite's module-resolution step before a test gets a
// chance to run. Aliased in vitest.config.ts so resolution always succeeds;
// a test that needs specific behavior still overrides individual methods
// with vi.mock or vi.spyOn against these exports.
import {
  mockAuthService,
  mockChatService,
  mockFlightService,
  mockFlightDataService,
  mockSettingsService,
  mockAudioService,
  mockDiscordService,
  mockUpdateService,
} from "./wails-bindings";

export const AuthService = mockAuthService();
export const ChatService = mockChatService();
export const FlightService = mockFlightService();
export const FlightDataService = mockFlightDataService();
export const SettingsService = mockSettingsService();
export const AudioService = mockAudioService();
export const DiscordService = mockDiscordService();
export const UpdateService = mockUpdateService();
