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
  const ctxRef = useRef<AudioContext | null>(null);
  const gainRef = useRef<GainNode | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const queueRef = useRef<SoundInstruction[]>([]);
  const processingRef = useRef(false);

  function ensureAudioGraph() {
    if (!ctxRef.current) {
      const ctx = new AudioContext();
      const gain = ctx.createGain();
      gain.gain.value = volumeRef.current / 100;
      gain.connect(ctx.destination);
      ctxRef.current = ctx;
      gainRef.current = gain;
    }
    return { ctx: ctxRef.current, gain: gainRef.current! };
  }

  // Update volume via GainNode
  useEffect(() => {
    volumeRef.current = volume;
    if (gainRef.current) {
      gainRef.current.gain.value = volume / 100;
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
        (async () => {
          try {
            const { ctx, gain } = ensureAudioGraph();
            if (ctx.state === "suspended") await ctx.resume();

            const binary = atob(base64Data);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i++) {
              bytes[i] = binary.charCodeAt(i);
            }
            const blob = new Blob([bytes], { type: contentType });
            const url = URL.createObjectURL(blob);

            const audio = new Audio(url);
            audioRef.current = audio;

            // Route through GainNode for reliable volume control
            const source = ctx.createMediaElementSource(audio);
            source.connect(gain);

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
        })();
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

  // Close AudioContext on unmount
  useEffect(() => {
    return () => {
      ctxRef.current?.close();
      ctxRef.current = null;
      gainRef.current = null;
    };
  }, []);
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
