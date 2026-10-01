import { SettingsService } from "../../bindings/airspace-acars";
import { generateNotificationSound } from "./notification-sounds";

export interface AudioOutputDeviceInfo {
  deviceId: string;
  label: string;
  isDefault: boolean;
}

export const AUDIO_DEVICE_STORAGE_KEY = "acars_audio_device";

let cachedAudioDevice: string | null = null;

/**
 * Retrieve the current configured audio output device ID from module cache or localStorage.
 */
export function getSelectedAudioDevice(): string {
  if (cachedAudioDevice !== null) return cachedAudioDevice;
  if (typeof localStorage === "undefined") return "default";
  return localStorage.getItem(AUDIO_DEVICE_STORAGE_KEY) || "default";
}

/**
 * Hydrates the audio output device from backend settings once at application startup.
 */
export function hydrateAudioOutputDevice(deviceId?: string): void {
  const normalized = deviceId || "default";
  cachedAudioDevice = normalized;
  if (typeof localStorage !== "undefined") {
    localStorage.setItem(AUDIO_DEVICE_STORAGE_KEY, normalized);
  }
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent("acars-audio-device-changed", { detail: { deviceId: normalized } })
    );
  }
}

/**
 * Store the configured audio output device ID and notify active listeners.
 */
export async function setSelectedAudioDevice(deviceId: string): Promise<void> {
  const normalized = deviceId || "default";
  cachedAudioDevice = normalized;
  if (typeof localStorage !== "undefined") {
    localStorage.setItem(AUDIO_DEVICE_STORAGE_KEY, normalized);
  }

  // Persist to backend settings if possible
  try {
    const s = await SettingsService.GetSettings();
    if (s && s.audioOutputDevice !== normalized) {
      await SettingsService.UpdateSettings({ ...s, audioOutputDevice: normalized });
    }
  } catch (e) {
    console.warn("[audio-manager] Failed to persist audio output device to settings:", e);
  }

  // Dispatch custom event to notify all active AudioContexts and Audio elements
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent("acars-audio-device-changed", { detail: { deviceId: normalized } })
    );
  }
}

/**
 * Prompts the native OS/browser audio output picker (Chrome 110+ / modern WebView2)
 * if available, returning the selected device.
 */
export async function selectSystemAudioOutput(): Promise<AudioOutputDeviceInfo | null> {
  if (
    typeof navigator !== "undefined" &&
    typeof (navigator.mediaDevices as any)?.selectAudioOutput === "function"
  ) {
    try {
      const dev = await (navigator.mediaDevices as any).selectAudioOutput();
      if (dev && dev.kind === "audiooutput") {
        return {
          deviceId: dev.deviceId || "default",
          label: dev.label || dev.deviceId || "Audio",
          isDefault: dev.deviceId === "default" || dev.deviceId === "",
        };
      }
    } catch (err: any) {
      if (err?.name !== "AbortError" && err?.name !== "NotAllowedError") {
        console.warn("[audio-manager] selectAudioOutput error:", err);
      }
    }
  }
  return null;
}

/**
 * Whether the media permission unlock (see unlockOutputDeviceEnumeration) has
 * already been attempted in this session and how it went. A denied attempt is
 * not retried automatically so a machine with the microphone privacy switch
 * off does not get hit on every dropdown open.
 */
let mediaPermissionState: "unknown" | "granted" | "denied" = "unknown";

/**
 * Chromium (WebView2 on Windows) only exposes audio output devices and their
 * labels to an origin that holds media (microphone) permission; without it,
 * enumerateDevices() returns at most a single unnamed "audiooutput" entry.
 * This is why the picker looked empty. There is no output-only permission in
 * Chromium, so the smallest possible microphone request is made: the stream
 * is stopped immediately and the microphone is never read. Wails answers the
 * WebView2 permission request itself, so no OS prompt is shown; the OS
 * microphone indicator and privacy log still record the app, which is why
 * the request is only ever made from the pilot's explicit "refresh devices"
 * action (see getAudioOutputDevices), never from opening the Settings tab
 * or the device dropdown.
 */
function isEnumerationLocked(outputDevices: MediaDeviceInfo[]): boolean {
  return outputDevices.length === 0 || outputDevices.some((d) => !d.label?.trim());
}

async function unlockOutputDeviceEnumeration(retryIfDenied: boolean): Promise<boolean> {
  if (mediaPermissionState === "granted") return true;
  if (mediaPermissionState === "denied" && !retryIfDenied) return false;
  if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
    return false;
  }

  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    for (const track of stream.getTracks()) {
      try {
        track.stop();
      } catch {
        // ignore
      }
    }
    mediaPermissionState = "granted";
    return true;
  } catch (err) {
    mediaPermissionState = "denied";
    console.debug("[audio-manager] media permission unlock for device enumeration failed:", err);
    return false;
  }
}

/** Test hook: forget the outcome of the media permission unlock. */
export function resetMediaPermissionStateForTests(): void {
  mediaPermissionState = "unknown";
}

export interface GetAudioOutputDevicesOptions {
  /**
   * Request media permission when the enumeration comes back locked
   * (Chromium only reveals output devices to origins holding it). Defaults to
   * false: the microphone is touched only on the pilot's explicit request.
   */
  requestPermission?: boolean;
  /**
   * Retry the permission request even if a previous attempt in this session
   * was denied. Used by the explicit "refresh devices" action.
   */
  retryIfDenied?: boolean;
}

/**
 * Enumerate all available audio output devices on the operating system.
 * Works across Windows (WASAPI / WebView2) and macOS (CoreAudio / WebKit).
 *
 * On Chromium the full list is only available once the origin holds media
 * permission. A one-off, immediately-stopped microphone request unlocks it
 * (see unlockOutputDeviceEnumeration), but only when the caller opts in with
 * `requestPermission`, which the Settings tab does for the explicit refresh
 * button alone. Automatic enumerations (tab mount, dropdown open, a device
 * being plugged in) never touch the microphone; once permission has been
 * granted they get the full list anyway.
 */
export async function getAudioOutputDevices(
  defaultLabel = "System Default",
  fallbackDeviceLabelPrefix = "Audio Output Device",
  options: GetAudioOutputDevicesOptions = {}
): Promise<AudioOutputDeviceInfo[]> {
  const { requestPermission = false, retryIfDenied = false } = options;
  const result: AudioOutputDeviceInfo[] = [];

  if (typeof navigator !== "undefined" && navigator.mediaDevices?.enumerateDevices) {
    try {
      let devices = await navigator.mediaDevices.enumerateDevices();
      let outputDevices = devices.filter((d) => d.kind === "audiooutput");

      if (requestPermission && isEnumerationLocked(outputDevices)) {
        const unlocked = await unlockOutputDeviceEnumeration(retryIfDenied);
        if (unlocked) {
          devices = await navigator.mediaDevices.enumerateDevices();
          outputDevices = devices.filter((d) => d.kind === "audiooutput");
        }
      }

      let unnamedCount = 1;
      for (const d of outputDevices) {
        const isDefault = d.deviceId === "default" || d.deviceId === "";
        const rawLabel = d.label?.trim() || "";

        let label = rawLabel;
        if (!label) {
          label = isDefault ? defaultLabel : `${fallbackDeviceLabelPrefix} ${unnamedCount++}`;
        }

        // Normalize deviceId for the default device
        const id = isDefault ? "default" : d.deviceId;

        // Avoid duplicates if default appears twice
        if (!result.some((r) => r.deviceId === id)) {
          result.push({
            deviceId: id,
            label,
            isDefault,
          });
        }
      }
    } catch (e) {
      console.warn("[audio-manager] enumerateDevices failed:", e);
    }
  }

  // Ensure default entry is always present at index 0
  if (!result.some((r) => r.isDefault || r.deviceId === "default")) {
    result.unshift({
      deviceId: "default",
      label: defaultLabel,
      isDefault: true,
    });
  }

  return result;
}

/**
 * Apply the selected audio output device to an HTMLAudioElement or AudioContext.
 * Uses W3C setSinkId supported natively on Chromium (WebView2 Windows) and Safari/WebKit (macOS).
 */
export async function applyAudioSink(
  target: HTMLAudioElement | AudioContext | null | undefined,
  targetDeviceId?: string
): Promise<boolean> {
  if (!target) return false;

  const deviceId = targetDeviceId !== undefined ? targetDeviceId : getSelectedAudioDevice();
  // In the W3C spec, setting sinkId to "" routes to system default output
  const sinkId = deviceId === "default" ? "" : deviceId;

  if (typeof (target as any).setSinkId === "function") {
    try {
      await (target as any).setSinkId(sinkId);
      return true;
    } catch (err) {
      console.warn(`[audio-manager] Failed to route audio to sink '${sinkId}':`, err);
      // If target device is no longer reachable (e.g. unplugged), fallback to default
      if (sinkId !== "") {
        try {
          await (target as any).setSinkId("");
        } catch {
          // ignore fallback error
        }
      }
      return false;
    }
  }

  return false;
}

/**
 * Creates an HTMLAudioElement configured with the user's selected audio output device and volume.
 */
export async function createManagedAudio(
  urlOrBlob: string | Blob,
  volume = 1
): Promise<{ audio: HTMLAudioElement; cleanup: () => void }> {
  const isBlob = typeof urlOrBlob !== "string";
  const url = isBlob ? URL.createObjectURL(urlOrBlob) : urlOrBlob;
  const audio = new Audio(url);

  audio.volume = Math.max(0, Math.min(1, volume));

  // Route to configured output device
  await applyAudioSink(audio);

  const cleanup = () => {
    if (isBlob) {
      URL.revokeObjectURL(url);
    }
  };

  audio.addEventListener("ended", cleanup, { once: true });
  audio.addEventListener("error", cleanup, { once: true });

  return { audio, cleanup };
}

/**
 * Subscribes to device changes (both internal app changes and OS-level hardware plug/unplug).
 */
export function subscribeAudioDeviceChange(callback: (deviceId: string) => void): () => void {
  if (typeof window === "undefined") return () => {};

  const handleDeviceChange = (e: Event) => {
    const detail = (e as CustomEvent)?.detail;
    callback(detail?.deviceId ?? getSelectedAudioDevice());
  };

  window.addEventListener("acars-audio-device-changed", handleDeviceChange);

  if (typeof navigator !== "undefined" && navigator.mediaDevices?.addEventListener) {
    navigator.mediaDevices.addEventListener("devicechange", handleDeviceChange);
  }

  return () => {
    window.removeEventListener("acars-audio-device-changed", handleDeviceChange);
    if (typeof navigator !== "undefined" && navigator.mediaDevices?.removeEventListener) {
      navigator.mediaDevices.removeEventListener("devicechange", handleDeviceChange);
    }
  };
}

/**
 * Plays a test sound on the chosen or currently configured audio output device.
 */
export async function playDeviceTestSound(
  deviceId?: string,
  volumePercent = 60
): Promise<void> {
  if (volumePercent <= 0) return;
  const blob = generateNotificationSound("chime");
  if (!blob) return;

  const volume = Math.max(0, Math.min(1, volumePercent / 100));
  const { audio } = await createManagedAudio(blob, volume);

  if (deviceId !== undefined) {
    await applyAudioSink(audio, deviceId);
  }

  try {
    await audio.play();
  } catch (e) {
    console.warn("[audio-manager] Test sound play prevented:", e);
  }
}
