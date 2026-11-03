import { useEffect, useRef } from "react";
import { Events } from "@wailsio/runtime";
import { AttendantService, AudioService } from "../../bindings/airspace-acars";
import { applyAudioSink, subscribeAudioDeviceChange } from "@/lib/audio-manager";
import type { AttendantClip } from "@/lib/attendant";
import { abortableSleep, playClip } from "@/lib/clip-player";

interface InterruptEvent {
  data: string;
}

/**
 * Plays the flight attendant's clips. The backend keeps the queue and decides
 * what plays when; this pulls the next clip when told one is ready, plays it
 * through the cabin volume and output device, and reports how it ended, which
 * is what boarding repeats are timed from. It lives in the app shell so it
 * keeps playing whichever tab is open.
 */
export function useAttendantPlayer(volume: number, active: boolean) {
  const volumeRef = useRef(volume);
  const ctxRef = useRef<AudioContext | null>(null);
  const gainRef = useRef<GainNode | null>(null);

  function audioGraph() {
    if (!ctxRef.current) {
      const ctx = new AudioContext();
      applyAudioSink(ctx);
      const gain = ctx.createGain();
      gain.gain.value = volumeRef.current / 100;
      gain.connect(ctx.destination);
      ctxRef.current = ctx;
      gainRef.current = gain;
    }
    return { ctx: ctxRef.current, gain: gainRef.current! };
  }

  useEffect(() => subscribeAudioDeviceChange((deviceId) => {
    if (ctxRef.current) applyAudioSink(ctxRef.current, deviceId);
  }), []);

  useEffect(() => {
    volumeRef.current = volume;
    if (gainRef.current) gainRef.current.gain.value = volume / 100;
  }, [volume]);

  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    let busy = false;
    let current: { id: string; abort: AbortController } | null = null;

    function playAudio(base64: string, contentType: string, signal: AbortSignal): Promise<boolean> {
      return new Promise((resolve) => {
        (async () => {
          try {
            const { ctx, gain } = audioGraph();
            if (ctx.state === "suspended") await ctx.resume();
            const binary = atob(base64);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
            const url = URL.createObjectURL(new Blob([bytes], { type: contentType }));
            const audio = new Audio(url);
            ctx.createMediaElementSource(audio).connect(gain);
            const finish = (ok: boolean) => {
              audio.pause();
              URL.revokeObjectURL(url);
              signal.removeEventListener("abort", onAbort);
              resolve(ok);
            };
            const onAbort = () => finish(false);
            signal.addEventListener("abort", onAbort, { once: true });
            audio.addEventListener("ended", () => finish(true));
            audio.addEventListener("error", () => finish(false));
            audio.play().catch(() => finish(false));
          } catch {
            resolve(false);
          }
        })();
      });
    }

    async function pump() {
      if (busy) return;
      busy = true;
      try {
        while (!cancelled) {
          const clip = (await AttendantService.TakeClip()) as unknown as AttendantClip | null;
          if (!clip || cancelled) break;
          const abort = new AbortController();
          current = { id: clip.id, abort };
          const outcome = await playClip(clip, {
            getAudio: async (file) => {
              const a = (await AudioService.GetAudioData(file)) as { data: string; contentType: string } | null;
              return a ? { data: a.data, contentType: a.contentType } : null;
            },
            playAudio,
            sleep: abortableSleep,
          }, abort.signal);
          current = null;
          // An interrupted clip was already recorded by the backend.
          if (outcome === "played") await AttendantService.ClipFinished(clip.id);
          else if (outcome === "failed") await AttendantService.ClipFailed(clip.id);
        }
      } catch {
        // The next clip-ready event tries again.
      } finally {
        busy = false;
      }
    }

    const offReady = Events.On("attendant-clip-ready", () => { void pump(); });
    const offInterrupt = Events.On("attendant-interrupt", (event: InterruptEvent) => {
      if (current && current.id === event?.data) current.abort.abort();
    });
    void pump();

    return () => {
      cancelled = true;
      current?.abort.abort();
      offReady();
      offInterrupt();
    };
  }, [active]);

  useEffect(() => () => {
    ctxRef.current?.close();
    ctxRef.current = null;
    gainRef.current = null;
  }, []);
}
