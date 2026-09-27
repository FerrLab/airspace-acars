# 📡 Airspace ACARS v1.20260927.126

**A big one for X-Plane pilots.** Three flight-data readings were plain wrong, and the server stored and scored them that way. If you fly X-Plane, please update.

## ✈️ X-Plane fixes

**Pitch and bank were inverted.** Every X-Plane flight was drawn with the attitude flipped — a climb showed as a descent, a right turn as a left one, both on the flight's PFD trace and in the 3D replay. Fixed.

**The flight director was being counted as the autopilot.** If you hand-fly with the FD on, that time was logged as autopilot time. It now only counts when the autopilot is actually flying — so your manual flying time is credited properly from here on.

**Armed speedbrakes were reported as a huge negative deflection.** X-Plane reports "armed" as `-0.5`, which isn't a deflection at all, and it ended up stored as -5000%. Armed now reads as stowed.

## 🛩️ Rotate MD-11 (X-Plane) — flaps now recorded

The Rotate MD-11 never reported a flap position, so **every approach failed the stabilised-approach check with the flaps "at unknown"**. There's now a profile that reads the aircraft's own flap handle.

Two things to be aware of: the handle variable and its scale couldn't be verified without the aircraft, so **if your flap percentages look off on this aircraft, please tell us** — it's a one-line correction. And no other aircraft is affected: if the variable isn't there, the stock reading is used exactly as before.

That brings profile coverage to **37 aircraft**.

## 🔐 Sign-in: bounced back to airline selection

If you entered your code, got in for half a second and were thrown back to the airline picker — **especially when switching from one airline to another** — that's fixed. The app was handing the server your new token while still pointing at the previous airline's address, and a token is only valid for the airline that issued it. The rejection signed you straight back out.

The workaround some of you found (searching for the airline instead of picking it from the recommendations) is no longer needed.

We've also added the sign-in logging that was missing entirely, so if anything like this happens again the log will actually show it. **Your token is never written to the log** — only whether one exists.

Two simulator log lines were misleading too: the app used to log "X-Plane UDP connected" even when X-Plane wasn't running at all (UDP can't actually confirm anything is there), and the "no data received from X-Plane" warning was hidden at debug level. Both now say what's really happening — so "is my sim running?" is answerable from the log.

## 🪟 Windows installer

The install folder is now writable by standard users, so updates apply without an admin prompt.

---

**Update:** the ACARS updates itself on launch, or grab `v1.20260927.126` from the downloads page.

Found something off? Post in the support channel with your log and we'll take a look. 🛠️
