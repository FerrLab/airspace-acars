export const CHAT_SOUNDS = ["default", "chime", "ding", "soft", "none"] as const;
export type ChatSoundType = (typeof CHAT_SOUNDS)[number];

export const CHAT_SOUND_LABELS: Record<ChatSoundType, string> = {
  default: "Default",
  chime: "Chime",
  ding: "Ding",
  soft: "Soft",
  none: "None",
};

export function generateNotificationSound(type: ChatSoundType): Blob | null {
  if (type === "none") return null;

  const sampleRate = 44100;
  let samples: Float32Array;

  switch (type) {
    case "chime": {
      const dur = 0.3;
      samples = new Float32Array(sampleRate * dur);
      for (let i = 0; i < samples.length; i++) {
        const t = i / sampleRate;
        if (t < 0.15) {
          samples[i] = Math.sin(2 * Math.PI * 523 * t) * Math.exp(-t * 12) * 0.3;
        } else {
          const t2 = t - 0.15;
          samples[i] =
            Math.sin(2 * Math.PI * 784 * t2) * Math.exp(-t2 * 12) * 0.3;
        }
      }
      break;
    }
    case "ding": {
      const dur = 0.3;
      samples = new Float32Array(sampleRate * dur);
      for (let i = 0; i < samples.length; i++) {
        const t = i / sampleRate;
        samples[i] =
          Math.sin(2 * Math.PI * 1047 * t) * Math.exp(-t * 8) * 0.25;
      }
      break;
    }
    case "soft": {
      const dur = 0.25;
      samples = new Float32Array(sampleRate * dur);
      for (let i = 0; i < samples.length; i++) {
        const t = i / sampleRate;
        samples[i] =
          Math.sin(2 * Math.PI * 440 * t) * Math.exp(-t * 10) * 0.2;
      }
      break;
    }
    default: {
      const dur = 0.15;
      samples = new Float32Array(sampleRate * dur);
      for (let i = 0; i < samples.length; i++) {
        const t = i / sampleRate;
        samples[i] =
          Math.sin(2 * Math.PI * 880 * t) * Math.exp(-t * 20) * 0.3;
      }
      break;
    }
  }

  return encodeWav(samples, sampleRate);
}

function encodeWav(samples: Float32Array, sampleRate: number): Blob {
  const numFrames = samples.length;
  const buf = new ArrayBuffer(44 + numFrames * 2);
  const view = new DataView(buf);
  const w = (off: number, s: string) => {
    for (let i = 0; i < s.length; i++) view.setUint8(off + i, s.charCodeAt(i));
  };
  w(0, "RIFF");
  view.setUint32(4, 36 + numFrames * 2, true);
  w(8, "WAVE");
  w(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  w(36, "data");
  view.setUint32(40, numFrames * 2, true);
  for (let i = 0; i < numFrames; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return new Blob([buf], { type: "audio/wav" });
}

export function playNotificationPreview(type: ChatSoundType) {
  const blob = generateNotificationSound(type);
  if (!blob) return;
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.play().catch(() => {});
  audio.addEventListener("ended", () => URL.revokeObjectURL(url));
}

// Two-tone cabin chime used when a flight auto-starts. Intentionally louder
// and longer than the chat notification sounds so it can't be missed.
function generateAutoStartDing(): Blob {
  const sampleRate = 44100;
  const tone1Dur = 0.28;
  const tone2Dur = 0.45;
  const gap = 0.04;
  const total = tone1Dur + gap + tone2Dur;
  const samples = new Float32Array(Math.floor(sampleRate * total));

  const writeTone = (offset: number, freq: number, dur: number, decay: number) => {
    const n = Math.floor(sampleRate * dur);
    for (let i = 0; i < n; i++) {
      const t = i / sampleRate;
      samples[offset + i] += Math.sin(2 * Math.PI * freq * t) * Math.exp(-t * decay) * 0.85;
    }
  };

  writeTone(0, 1047, tone1Dur, 6);
  writeTone(Math.floor(sampleRate * (tone1Dur + gap)), 784, tone2Dur, 4);

  return encodeWav(samples, sampleRate);
}

export function playAutoStartDing(volumePercent: number) {
  if (volumePercent <= 0) return;
  const blob = generateAutoStartDing();
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.volume = Math.min(1, volumePercent / 100);
  audio.play().catch(() => {});
  audio.addEventListener("ended", () => URL.revokeObjectURL(url));
}
