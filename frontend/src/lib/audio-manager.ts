import { SettingsService } from "../../bindings/airspace-acars";
import { generateNotificationSound } from "./notification-sounds";

export interface AudioOutputDeviceInfo {
  deviceId: string;
  label: string;
  isDefault: boolean;
}

export const AUDIO_DEVICE_STORAGE_KEY = "acars_audio_device";

/**
 * Retrieve the current configured audio output device ID from localStorage or settings.
 */
export function getSelectedAudioDevice(): string {
  if (typeof localStorage === "undefined") return "default";
  return localStorage.getItem(AUDIO_DEVICE_STORAGE_KEY) || "default";
}

/**
 * Store the configured audio output device ID and notify active listeners.
 */
export async function setSelectedAudioDevice(deviceId: string): Promise<void> {
  const normalized = deviceId || "default";
  if (typeof localStorage !== "undefined") {
    localStorage.setItem(AUDIO_DEVICE_STORAGE_KEY, normalized);
  }

  // Persist to backend settings if possible
  try {
    const s = await SettingsService.GetSettings();
    if (s && s.audioOutputDevice !== normalized) {
      await SettingsService.UpdateSettings({ ...s, audioOutputDevice: normalized });
    }
  } catch {
    // ignore backend sync error
  }

  // Dispatch custom event to notify all active AudioContexts and Audio elements
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent("acars-audio-device-changed", { detail: { deviceId: normalized } })
    );
  }
}

let hasRequestedPermissions = false;

/**
 * Prompt/unlock media permissions in WebView2 / Chromium.
 * Once granted, navigator.mediaDevices.enumerateDevices() provides full device
 * labels and non-default device identifiers.
 */
export async function requestMediaPermissions(): Promise<boolean> {
  if (typeof navigator === "undefined" || !navigator.mediaDevices?.getUserMedia) {
    return false;
  }

  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    stream.getTracks().forEach((track) => {
      try {
        track.stop();
      } catch {
        // ignore
      }
    });
    hasRequestedPermissions = true;
    return true;
  } catch (err) {
    console.debug("[audio-manager] getUserMedia permission request:", err);
    return false;
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
          label: dev.label || dev.deviceId || "Áudio",
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
 * Enumerate all available audio output devices on the operating system.
 * Works seamlessly across Windows (WASAPI / WebView2) and macOS (CoreAudio / WebKit).
 */
export async function getAudioOutputDevices(
  defaultLabel = "Padrão do Sistema",
  autoRequestPermission = true
): Promise<AudioOutputDeviceInfo[]> {
  const result: AudioOutputDeviceInfo[] = [];

  if (typeof navigator !== "undefined" && navigator.mediaDevices?.enumerateDevices) {
    try {
      let devices = await navigator.mediaDevices.enumerateDevices();
      let outputDevices = devices.filter((d) => d.kind === "audiooutput");

      // In Chromium / WebView2, if permissions have not been granted,
      // outputDevices will either contain only 1 item (default) or have empty labels.
      const needsUnlock =
        autoRequestPermission &&
        !hasRequestedPermissions &&
        (outputDevices.length <= 1 || outputDevices.some((d) => !d.label));

      if (needsUnlock && navigator.mediaDevices.getUserMedia) {
        const ok = await requestMediaPermissions();
        if (ok) {
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
          label = isDefault ? defaultLabel : `Dispositivo de Áudio ${unnamedCount++}`;
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
  if (!result.some((d) => d.deviceId === "default" || d.isDefault)) {
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
  const blob = generateNotificationSound("chime");
  if (!blob) return;

  const volume = Math.min(1, Math.max(0.1, volumePercent / 100));
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
