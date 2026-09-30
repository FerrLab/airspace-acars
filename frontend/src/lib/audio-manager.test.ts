import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  getSelectedAudioDevice,
  setSelectedAudioDevice,
  hydrateAudioOutputDevice,
  playDeviceTestSound,
  getAudioOutputDevices,
  applyAudioSink,
  subscribeAudioDeviceChange,
  resetMediaPermissionStateForTests,
  AUDIO_DEVICE_STORAGE_KEY,
} from "./audio-manager";

describe("audio-manager", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
    resetMediaPermissionStateForTests();
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

  it("does not request microphone access when output labels are already exposed", async () => {
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

  it("unlocks the full output list with a one-off, stopped media request when Chromium hides it", async () => {
    // Without media permission Chromium returns a single unnamed output device.
    const stopMock = vi.fn();
    const getUserMediaMock = vi.fn().mockResolvedValue({
      getTracks: () => [{ stop: stopMock }],
    });
    const enumerateDevicesMock = vi
      .fn()
      .mockResolvedValueOnce([{ deviceId: "", kind: "audiooutput", label: "" }])
      .mockResolvedValue([
        { deviceId: "default", kind: "audiooutput", label: "Default - Realtek" },
        { deviceId: "communications", kind: "audiooutput", label: "Communications - Realtek" },
        { deviceId: "headset-1", kind: "audiooutput", label: "USB Headset" },
        { deviceId: "mic-1", kind: "audioinput", label: "Microphone" },
      ]);

    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia: getUserMediaMock,
        enumerateDevices: enumerateDevicesMock,
      },
    });

    const devices = await getAudioOutputDevices("System Default");
    expect(getUserMediaMock).toHaveBeenCalledWith({ audio: true });
    expect(stopMock).toHaveBeenCalled();
    expect(enumerateDevicesMock).toHaveBeenCalledTimes(2);
    expect(devices.map((d) => d.deviceId)).toEqual(["default", "communications", "headset-1"]);
    expect(devices[2].label).toBe("USB Headset");

    // Once granted, later enumerations already carry labels and never touch the mic again.
    await getAudioOutputDevices("System Default");
    expect(getUserMediaMock).toHaveBeenCalledTimes(1);
  });

  it("falls back to the default entry and does not retry when the media request is denied", async () => {
    const getUserMediaMock = vi.fn().mockRejectedValue(Object.assign(new Error("denied"), { name: "NotAllowedError" }));
    const enumerateDevicesMock = vi
      .fn()
      .mockResolvedValue([{ deviceId: "", kind: "audiooutput", label: "" }]);

    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia: getUserMediaMock,
        enumerateDevices: enumerateDevicesMock,
      },
    });

    const devices = await getAudioOutputDevices("System Default");
    expect(devices).toEqual([{ deviceId: "default", label: "System Default", isDefault: true }]);
    expect(getUserMediaMock).toHaveBeenCalledTimes(1);

    // Automatic enumerations (dropdown open, mount) do not nag the OS again...
    await getAudioOutputDevices("System Default");
    expect(getUserMediaMock).toHaveBeenCalledTimes(1);

    // ...but an explicit refresh retries.
    await getAudioOutputDevices("System Default", "Audio Output Device", { retryIfDenied: true });
    expect(getUserMediaMock).toHaveBeenCalledTimes(2);
  });

  it("skips the media request when requestPermission is false", async () => {
    const getUserMediaMock = vi.fn();
    vi.stubGlobal("navigator", {
      mediaDevices: {
        getUserMedia: getUserMediaMock,
        enumerateDevices: vi.fn().mockResolvedValue([{ deviceId: "", kind: "audiooutput", label: "" }]),
      },
    });

    const devices = await getAudioOutputDevices("System Default", "Audio Output Device", {
      requestPermission: false,
    });
    expect(getUserMediaMock).not.toHaveBeenCalled();
    expect(devices).toHaveLength(1);
    expect(devices[0].deviceId).toBe("default");
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


