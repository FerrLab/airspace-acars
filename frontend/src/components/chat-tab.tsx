import { useState, useEffect, useRef, useCallback } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Send, Plane, ChevronDown } from "lucide-react";
import { ChatService } from "../../bindings/airspace-acars";

interface Message {
  id: number;
  senderId: number;
  senderName: string;
  senderRole: string | null;
  type: string;
  text: string;
  timestamp: string;
  read: boolean;
}

type Sender = "user" | "other" | "acars";

function classifySender(msg: Message, myUserId: number | null): Sender {
  if (msg.type === "acars") return "acars";
  if (myUserId !== null && msg.senderId === myUserId) return "user";
  return "other";
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  } catch {
    return iso;
  }
}

function usePingSound() {
  const audioRef = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    // Generate a short ping tone as a data URL
    const ctx = new AudioContext();
    const duration = 0.15;
    const sampleRate = ctx.sampleRate;
    const buffer = ctx.createBuffer(1, sampleRate * duration, sampleRate);
    const data = buffer.getChannelData(0);
    for (let i = 0; i < data.length; i++) {
      const t = i / sampleRate;
      data[i] = Math.sin(2 * Math.PI * 880 * t) * Math.exp(-t * 20) * 0.3;
    }
    // Encode to wav data URL
    const numFrames = buffer.length;
    const wavBuf = new ArrayBuffer(44 + numFrames * 2);
    const view = new DataView(wavBuf);
    const writeStr = (offset: number, s: string) => {
      for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
    };
    writeStr(0, "RIFF");
    view.setUint32(4, 36 + numFrames * 2, true);
    writeStr(8, "WAVE");
    writeStr(12, "fmt ");
    view.setUint32(16, 16, true);
    view.setUint16(20, 1, true);
    view.setUint16(22, 1, true);
    view.setUint32(24, sampleRate, true);
    view.setUint32(28, sampleRate * 2, true);
    view.setUint16(32, 2, true);
    view.setUint16(34, 16, true);
    writeStr(36, "data");
    view.setUint32(40, numFrames * 2, true);
    for (let i = 0; i < numFrames; i++) {
      const s = Math.max(-1, Math.min(1, data[i]));
      view.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true);
    }
    const blob = new Blob([wavBuf], { type: "audio/wav" });
    const url = URL.createObjectURL(blob);
    audioRef.current = new Audio(url);
    ctx.close();

    return () => URL.revokeObjectURL(url);
  }, []);

  return useCallback(() => {
    if (audioRef.current) {
      audioRef.current.currentTime = 0;
      audioRef.current.play().catch(() => {});
    }
  }, []);
}

interface ChatTabProps {
  localMode?: boolean;
}

export function ChatTab({ localMode = false }: ChatTabProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [myUserId, setMyUserId] = useState<number | null>(() => {
    const stored = localStorage.getItem("acars_chat_user_id");
    return stored ? parseInt(stored, 10) : null;
  });
  const [lastPage, setLastPage] = useState(1);
  const [loadingMore, setLoadingMore] = useState(false);
  const [showScrollBtn, setShowScrollBtn] = useState(false);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const prevScrollHeightRef = useRef(0);
  const initialLoadDone = useRef(false);
  const playPing = usePingSound();

  const scrollToBottom = useCallback((smooth = true) => {
    messagesEndRef.current?.scrollIntoView({
      behavior: smooth ? "smooth" : "instant",
    });
  }, []);

  // Persist myUserId to localStorage
  useEffect(() => {
    if (myUserId !== null) {
      localStorage.setItem("acars_chat_user_id", String(myUserId));
    }
  }, [myUserId]);

  // Fetch latest messages (page 1) on mount and poll every 5s
  useEffect(() => {
    if (localMode) return;
    let active = true;

    async function fetchLatest() {
      try {
        const resp = await ChatService.GetMessages(1);
        if (!active || !resp) return;

        setLastPage(resp.last_page);
        const incoming = (resp.data ?? []).map(mapMessage);

        setMessages((prev) => {
          const existingIds = new Set(prev.map((m) => m.id));
          const newMsgs = incoming.filter((m) => !existingIds.has(m.id));
          if (newMsgs.length === 0) return prev;

          // Ping if new messages from others arrived (not initial load)
          if (initialLoadDone.current && newMsgs.some((m) => myUserId === null || m.senderId !== myUserId)) {
            playPing();
          }

          return [...prev, ...newMsgs];
        });

        // Auto-confirm unread messages from others
        for (const msg of incoming) {
          if (!msg.read && myUserId !== null && msg.senderId !== myUserId) {
            ChatService.ConfirmMessage(msg.id).catch(() => {});
          }
        }
      } catch {
        // ignore polling errors
      }
    }

    fetchLatest().then(() => {
      if (!initialLoadDone.current) {
        initialLoadDone.current = true;
        setTimeout(() => scrollToBottom(false), 100);
      }
    });

    const interval = setInterval(fetchLatest, 5000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, [localMode, myUserId, scrollToBottom, playPing]);

  // Auto-scroll when new messages appear
  useEffect(() => {
    if (!initialLoadDone.current) return;
    const container = scrollContainerRef.current;
    if (!container) return;
    const nearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight < 100;
    if (nearBottom) {
      scrollToBottom();
    } else {
      setShowScrollBtn(true);
    }
  }, [messages.length, scrollToBottom]);

  // Infinite scroll: load older pages when scrolled to top
  useEffect(() => {
    const sentinel = sentinelRef.current;
    const container = scrollContainerRef.current;
    if (!sentinel || !container) return;

    const observer = new IntersectionObserver(
      async ([entry]) => {
        if (!entry.isIntersecting || loadingMore) return;

        // Figure out which page to load next
        const currentPages = Math.ceil(messages.length / 15); // assume ~15 per page
        const nextPage = currentPages + 1;
        if (nextPage > lastPage) return;

        setLoadingMore(true);
        prevScrollHeightRef.current = container.scrollHeight;

        try {
          const resp = await ChatService.GetMessages(nextPage);
          if (!resp) return;

          const older = (resp.data ?? []).map(mapMessage);
          setMessages((prev) => {
            const existingIds = new Set(prev.map((m) => m.id));
            const newMsgs = older.filter((m) => !existingIds.has(m.id));
            return [...newMsgs, ...prev];
          });

          // Preserve scroll position
          requestAnimationFrame(() => {
            const newHeight = container.scrollHeight;
            container.scrollTop += newHeight - prevScrollHeightRef.current;
          });
        } catch {
          // ignore
        } finally {
          setLoadingMore(false);
        }
      },
      { root: container, threshold: 0.1 }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [messages.length, lastPage, loadingMore]);

  // Track scroll position for "new messages" button
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    function onScroll() {
      const nearBottom =
        container!.scrollHeight - container!.scrollTop - container!.clientHeight <
        100;
      if (nearBottom) setShowScrollBtn(false);
    }

    container.addEventListener("scroll", onScroll);
    return () => container.removeEventListener("scroll", onScroll);
  }, []);

  async function handleSend() {
    const text = input.trim();
    if (!text) return;

    setSending(true);
    try {
      const result = await ChatService.SendMessage(text);
      setInput("");

      if (result?.id) {
        const msg = mapMessage(result);
        if (msg.senderId) setMyUserId(msg.senderId);
        setMessages((prev) => {
          if (prev.some((m) => m.id === msg.id)) return prev;
          return [...prev, msg];
        });
        setTimeout(() => scrollToBottom(), 50);
      }
    } catch (e) {
      console.error("Failed to send message:", e);
    } finally {
      setSending(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  const sorted = [...messages].sort((a, b) => a.id - b.id);

  if (localMode) {
    return (
      <div className="flex h-full flex-col items-center justify-center">
        <div className="text-center space-y-2">
          <Badge variant="outline" className="border-yellow-500/50 text-yellow-500">
            Local Mode
          </Badge>
          <h2 className="text-lg font-semibold tracking-tight">Chat</h2>
          <p className="text-sm text-muted-foreground">
            Chat is unavailable in local mode
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between pb-4">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Chat</h2>
          <p className="text-sm text-muted-foreground">
            Dispatch communication
          </p>
        </div>
        <span className="text-xs text-muted-foreground tabular-nums">
          {messages.length} messages
        </span>
      </div>

      <Separator />

      <div className="relative flex-1 min-h-0 mt-4">
        <div
          ref={scrollContainerRef}
          className="absolute inset-0 overflow-y-auto space-y-2 px-1"
        >
          <div ref={sentinelRef} className="h-1" />
          {loadingMore && (
            <p className="text-center text-xs text-muted-foreground py-2">
              Loading older messages...
            </p>
          )}
          {sorted.length === 0 && (
            <p className="text-center text-sm text-muted-foreground py-8">
              No messages yet
            </p>
          )}
          {sorted.map((msg) => {
            const sender = classifySender(msg, myUserId);
            return (
              <ChatBubble key={msg.id} message={msg} sender={sender} />
            );
          })}
          <div ref={messagesEndRef} />
        </div>

        {showScrollBtn && (
          <button
            onClick={() => {
              scrollToBottom();
              setShowScrollBtn(false);
            }}
            className="absolute bottom-2 left-1/2 -translate-x-1/2 flex items-center gap-1 rounded-full bg-primary px-3 py-1 text-xs text-primary-foreground shadow-lg"
          >
            <ChevronDown className="h-3 w-3" />
            New messages
          </button>
        )}
      </div>

      <div className="flex items-center gap-2 pt-4">
        <Input
          placeholder="Type a message..."
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={sending}
          className="flex-1"
        />
        <Button size="sm" onClick={handleSend} disabled={sending || !input.trim()}>
          <Send className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function mapMessage(raw: any): Message {
  return {
    id: raw.id,
    senderId: raw.sender_id ?? raw.senderId ?? 0,
    senderName: raw.sender_name ?? raw.senderName ?? "",
    senderRole: raw.sender_role ?? raw.senderRole ?? null,
    type: raw.type ?? "",
    text: raw.message ?? raw.text ?? "",
    timestamp: raw.created_at ?? raw.createdAt ?? "",
    read: raw.read_at != null,
  };
}

function ChatBubble({
  message,
  sender,
}: {
  message: Message;
  sender: Sender;
}) {
  if (sender === "acars") {
    return (
      <div className="flex justify-start">
        <div className="max-w-[75%] rounded-lg border border-border bg-primary/5 px-3 py-2">
          <div className="flex items-center gap-1.5 mb-1">
            <Plane className="h-3 w-3 text-primary" />
            <span className="text-[10px] font-medium text-primary">ACARS</span>
          </div>
          <p className="text-sm font-mono whitespace-pre-wrap">{message.text}</p>
          <span className="block text-[10px] text-muted-foreground mt-1">
            {formatTime(message.timestamp)}
          </span>
        </div>
      </div>
    );
  }

  if (sender === "user") {
    return (
      <div className="flex justify-end">
        <div className="max-w-[75%] rounded-lg bg-primary px-3 py-2 text-primary-foreground">
          <p className="text-sm whitespace-pre-wrap">{message.text}</p>
          <div className="flex items-center justify-end gap-1 mt-1">
            <span className="text-[10px] opacity-70">
              {formatTime(message.timestamp)}
            </span>
            {message.read && (
              <span className="text-[10px] opacity-70">✓✓</span>
            )}
          </div>
        </div>
      </div>
    );
  }

  // "other" - staff message
  return (
    <div className="flex justify-start">
      <div className="max-w-[75%] rounded-lg bg-muted px-3 py-2">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-xs font-medium">{message.senderName}</span>
          {message.senderRole && (
            <Badge variant="secondary" className="text-[9px] px-1 py-0">
              {message.senderRole}
            </Badge>
          )}
        </div>
        <p className="text-sm whitespace-pre-wrap">{message.text}</p>
        <span className="block text-[10px] text-muted-foreground mt-1">
          {formatTime(message.timestamp)}
        </span>
      </div>
    </div>
  );
}
