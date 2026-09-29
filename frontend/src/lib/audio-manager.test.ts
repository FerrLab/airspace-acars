import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  getSelectedAudioDevice,
  setSelectedAudioDevice,
  hydrateAudioOutputDevice,
  playDeviceTestSound,
  getAudioOutputDevices,
  applyAudioSink,
  subscribeAudioDeviceChange,
  AUDIO_DEVICE_STORAGE_KEY,
} from "./audio-manager";

describe("audio-manager", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("returns 'default' when no device has been selected", () => {
    expect(getSelectedAudioDevice()).toBe("default");
  });

  it("saves and retrieves the selected audio device", async () => {
    await setSelectedAudioDevice("headphones-device-123");
    expect(localStorage.getItem(AUDIO_DEVICE_STORAGE_KEY)).toBe("headphones-device-123");
    expect(getSelectedAudioDevice()).toBe("headphones-device-123");
  });

  it("returns a default device entry when navigator.mediaDevices is empty or mock", async () => {
    const devices = await getAudioOutputDevices("Padrão do Sistema");
    expect(devices.length).toBeGreaterThanOrEqual(1);
    expect(devices[0].deviceId).toBe("default");
    expect(devices[0].isDefault).toBe(true);
    expect(devices[0].label).toBe("Padrão do Sistema");
  });

  it("enumerates audio output devices from navigator.mediaDevices when available", async () => {
    const mockDevices: MediaDeviceInfo[] = [
      {
        deviceId: "default",
        groupId: "group-1",
        kind: "audiooutput",
        label: "Default - Speakers (Realtek)",
        toJSON: () => ({}),
      },
      {
        deviceId: "headset-456",
        groupId: "group-2",
        kind: "audiooutput",
        label: "Headphones (HyperX)",
        toJSON: () => ({}),
      },
      {
        deviceId: "mic-789",
        groupId: "group-3",
        kind: "audioinput",
        label: "Microphone (Realtek)",
        toJSON: () => ({}),
      },
    ];

    vi.stubGlobal("navigator", {
      mediaDevices: {
        enumerateDevices: vi.fn().mockResolvedValue(mockDevices),
      },
    });

    const devices = await getAudioOutputDevices();
    expect(devices).toHaveLength(2);
    expect(devices[0].deviceId).toBe("default");
    expect(devices[0].label).toBe("Default - Speakers (Realtek)");
    expect(devices[1].deviceId).toBe("headset-456");
    expect(devices[1].label).toBe("Headphones (HyperX)");
  });

  it("applies sink ID to target element if setSinkId function is present", async () => {
    const mockAudio: any = {
      setSinkId: vi.fn().mockResolvedValue(undefined),
    };

    const success = await applyAudioSink(mockAudio, "headset-456");
    expect(success).toBe(true);
    expect(mockAudio.setSinkId).toHaveBeenCalledWith("headset-456");
  });

  it("applies empty string sink ID for 'default' device per W3C specification", async () => {
    const mockAudio: any = {
      setSinkId: vi.fn().mockResolvedValue(undefined),
    };

    const success = await applyAudioSink(mockAudio, "default");
    expect(success).toBe(true);
    expect(mockAudio.setSinkId).toHaveBeenCalledWith("");
  });

  it("notifies listeners when audio device changes", async () => {
    const listener = vi.fn();
    const unsubscribe = subscribeAudioDeviceChange(listener);

    await setSelectedAudioDevice("virtual-cable-1");
    expect(listener).toHaveBeenCalledWith("virtual-cable-1");

    unsubscribe();
    await setSelectedAudioDevice("speakers-2");
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("enumerates output devices without requesting microphone access", async () => {
    const getUserMediaMock = vi.fn();
    const enumerateDevicesMock = vi.fn().mockResolvedValue([
      { deviceId: "default", kind: "audiooutput", label: "Default - Realtek" },
      { deviceId: "headset-1", kind: "audiooutput", label: "USB Headset" },
    ]);

    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia: getUserMediaMock,
        enumerateDevices: enumerateDevicesMock,
      },
    });

    const devices = await getAudioOutputDevices("System Default");
    expect(getUserMediaMock).not.toHaveBeenCalled();
    expect(devices).toHaveLength(2);
    expect(devices[1].deviceId).toBe("headset-1");
    expect(devices[1].label).toBe("USB Headset");
  });

  it("hydrates audio output device from backend settings and notifies listeners", () => {
    const listener = vi.fn();
    const unsub = subscribeAudioDeviceChange(listener);

    hydrateAudioOutputDevice("speakers-surround");
    expect(getSelectedAudioDevice()).toBe("speakers-surround");
    expect(localStorage.getItem(AUDIO_DEVICE_STORAGE_KEY)).toBe("speakers-surround");
    expect(listener).toHaveBeenCalledWith("speakers-surround");

    unsub();
  });

  it("formats unnamed devices with the provided fallback label prefix", async () => {
    const mockDevices: MediaDeviceInfo[] = [
      {
        deviceId: "unnamed-1",
        groupId: "group-1",
        kind: "audiooutput",
        label: "",
        toJSON: () => ({}),
      },
    ];

    vi.stubGlobal("navigator", {
      mediaDevices: {
        enumerateDevices: vi.fn().mockResolvedValue(mockDevices),
      },
    });

    const devices = await getAudioOutputDevices("System Default", "Dispositivo de Áudio");
    expect(devices).toHaveLength(2);
    expect(devices[0].deviceId).toBe("default");
    expect(devices[1].deviceId).toBe("unnamed-1");
    expect(devices[1].label).toBe("Dispositivo de Áudio 1");
  });

  it("does not play test sound when volume is 0 or less", async () => {
    const playSpy = vi.fn();
    vi.stubGlobal("Audio", vi.fn().mockImplementation(() => ({
      play: playSpy,
      addEventListener: vi.fn(),
      setSinkId: vi.fn().mockResolvedValue(undefined),
    })));

    await playDeviceTestSound("default", 0);
    expect(playSpy).not.toHaveBeenCalled();

    await playDeviceTestSound("default", -10);
    expect(playSpy).not.toHaveBeenCalled();
  });
});


