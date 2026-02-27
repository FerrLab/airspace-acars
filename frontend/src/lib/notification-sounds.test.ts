import { describe, it, expect } from "vitest";
import {
  generateNotificationSound,
  CHAT_SOUNDS,
  CHAT_SOUND_LABELS,
  type ChatSoundType,
} from "./notification-sounds";

describe("generateNotificationSound", () => {
  it("returns null for 'none'", () => {
    expect(generateNotificationSound("none")).toBeNull();
  });

  it.each(["default", "chime", "ding", "soft"] as const)(
    "returns a Blob for '%s'",
    (type_) => {
      const blob = generateNotificationSound(type_);
      expect(blob).toBeInstanceOf(Blob);
      expect(blob!.type).toBe("audio/wav");
    }
  );

  it("produces WAV with correct RIFF header", async () => {
    const blob = generateNotificationSound("default")!;
    const buffer = await blob.arrayBuffer();
    const view = new DataView(buffer);

    // "RIFF" magic
    const riff = String.fromCharCode(
      view.getUint8(0),
      view.getUint8(1),
      view.getUint8(2),
      view.getUint8(3)
    );
    expect(riff).toBe("RIFF");

    // "WAVE" format
    const wave = String.fromCharCode(
      view.getUint8(8),
      view.getUint8(9),
      view.getUint8(10),
      view.getUint8(11)
    );
    expect(wave).toBe("WAVE");

    // PCM format (1)
    expect(view.getUint16(20, true)).toBe(1);

    // Mono (1 channel)
    expect(view.getUint16(22, true)).toBe(1);

    // 44100 Hz sample rate
    expect(view.getUint32(24, true)).toBe(44100);
  });

  it("produces different durations for different types", async () => {
    const defaultBlob = generateNotificationSound("default")!;
    const chimeBlob = generateNotificationSound("chime")!;

    // "default" is 0.15s, "chime" is 0.3s — chime should be larger
    expect(chimeBlob.size).toBeGreaterThan(defaultBlob.size);
  });
});

describe("CHAT_SOUNDS", () => {
  it("contains expected sound types", () => {
    expect(CHAT_SOUNDS).toContain("default");
    expect(CHAT_SOUNDS).toContain("chime");
    expect(CHAT_SOUNDS).toContain("ding");
    expect(CHAT_SOUNDS).toContain("soft");
    expect(CHAT_SOUNDS).toContain("none");
    expect(CHAT_SOUNDS).toHaveLength(5);
  });
});

describe("CHAT_SOUND_LABELS", () => {
  it("has a label for every sound type", () => {
    for (const sound of CHAT_SOUNDS) {
      expect(CHAT_SOUND_LABELS[sound as ChatSoundType]).toBeDefined();
      expect(typeof CHAT_SOUND_LABELS[sound as ChatSoundType]).toBe("string");
    }
  });
});
