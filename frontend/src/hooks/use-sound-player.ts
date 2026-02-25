import { useEffect, useRef } from "react";
import { AudioService } from "../../bindings/airspace-acars";

interface SoundInstruction {
  type: string;
  url?: string;
  localFile?: string;
  duration_ms: number;
}

export function useSoundPlayer(volume: number, active: boolean) {
  const volumeRef = useRef(volume);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const queueRef = useRef<SoundInstruction[]>([]);
  const processingRef = useRef(false);

  useEffect(() => {
    volumeRef.current = volume;
    if (audioRef.current) {
      audioRef.current.volume = Math.min(volume / 100, 1);
    }
  }, [volume]);

  useEffect(() => {
    if (!active) return;

    let cancelled = false;

    async function poll() {
      while (!cancelled) {
        try {
          const instructions = await AudioService.FetchSoundInstructions();
          if (!cancelled && instructions && instructions.length > 0) {
            queueRef.current.push(...instructions);
            processQueue();
          }
        } catch {
          // ignore polling errors
        }
        await sleep(10_000);
      }
    }

    async function processQueue() {
      if (processingRef.current) return;
      processingRef.current = true;

      while (queueRef.current.length > 0 && !cancelled) {
        const inst = queueRef.current.shift()!;

        if (inst.type === "play" && inst.localFile) {
          try {
            const audioData = await AudioService.GetAudioData(inst.localFile);
            if (cancelled || !audioData) continue;
            await playAudio(audioData.data, audioData.contentType);
          } catch {
            // skip failed audio
          }
        } else if (inst.type === "pause") {
          await sleep(inst.duration_ms);
        }
      }

      processingRef.current = false;
    }

    function playAudio(base64Data: string, contentType: string): Promise<void> {
      return new Promise((resolve) => {
        try {
          const binary = atob(base64Data);
          const bytes = new Uint8Array(binary.length);
          for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
          }
          const blob = new Blob([bytes], { type: contentType });
          const url = URL.createObjectURL(blob);

          const audio = new Audio(url);
          audioRef.current = audio;
          audio.volume = Math.min(volumeRef.current / 100, 1);

          audio.addEventListener("ended", () => {
            URL.revokeObjectURL(url);
            audioRef.current = null;
            resolve();
          });

          audio.addEventListener("error", () => {
            URL.revokeObjectURL(url);
            audioRef.current = null;
            resolve();
          });

          audio.play().catch(() => {
            URL.revokeObjectURL(url);
            audioRef.current = null;
            resolve();
          });
        } catch {
          resolve();
        }
      });
    }

    poll();

    return () => {
      cancelled = true;
      if (audioRef.current) {
        audioRef.current.pause();
        audioRef.current = null;
      }
      queueRef.current = [];
      processingRef.current = false;
    };
  }, [active]);
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
