import { useState, useEffect, useRef, useCallback } from "react";
import { ChatService } from "../../bindings/airspace-acars";

export function useUnreadChat(isChatOpen: boolean, localMode = false) {
  const [hasUnread, setHasUnread] = useState(false);
  const lastSeenIdRef = useRef<number>(0);
  const initializedRef = useRef(false);

  // When the user opens the chat tab, clear the unread indicator
  useEffect(() => {
    if (isChatOpen) {
      setHasUnread(false);
    }
  }, [isChatOpen]);

  const markSeen = useCallback(() => {
    setHasUnread(false);
  }, []);

  // Poll for new messages (skip in local mode)
  useEffect(() => {
    if (localMode) return;
    let active = true;

    async function check() {
      try {
        const resp = await ChatService.GetMessages(1);
        if (!active || !resp?.data?.length) return;

        const maxId = Math.max(...resp.data.map((m: any) => m.id));

        if (!initializedRef.current) {
          // First poll: just record the current max ID, don't flag as unread
          lastSeenIdRef.current = maxId;
          initializedRef.current = true;
          return;
        }

        if (maxId > lastSeenIdRef.current) {
          lastSeenIdRef.current = maxId;
          if (!isChatOpen) {
            setHasUnread(true);
          }
        }
      } catch {
        // ignore
      }
    }

    check();
    const interval = setInterval(check, 5_000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, [localMode, isChatOpen]);

  return { hasUnread, markSeen };
}
