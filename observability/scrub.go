package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
)

// The ACARS holds a bearer token for the pilot's network, the URL of their
// tenant, and paths under their Windows profile. None of it is anyone else's
// business, and none of it helps debug a crash, so it is removed on the way
// out rather than trusted not to appear.

const redacted = "[redacted]"

var (
	// bearerRe catches an Authorization header or anything written like one.
	// The optional scheme in the middle matters: without it "Authorization:
	// Bearer <secret>" redacts the word "Bearer" and leaves the secret.
	bearerRe = regexp.MustCompile(`(?i)\b(?:authorization|bearer|token|api[_-]?key|secret|password)\b\s*[:=]?\s*(?:(?:bearer|basic|token)\s+)?\S+`)

	// jwtRe catches a bare JSON Web Token, which is what this API issues.
	jwtRe = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`)

	// longSecretRe catches the device codes and opaque keys passed around
	// during sign-in: a long run of token characters with no spaces.
	longSecretRe = regexp.MustCompile(`\b[A-Za-z0-9_-]{40,}\b`)

	// querySecretRe catches a secret handed over in a URL.
	querySecretRe = regexp.MustCompile(`(?i)([?&](?:token|code|secret|key|password|auth)=)[^&\s]+`)
)

// homePaths are the directories whose names carry the account name.
var homePaths = func() []string {
	var out []string
	for _, dir := range []string{os.Getenv("USERPROFILE"), os.Getenv("HOME"), os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
		if dir = strings.TrimSpace(dir); dir != "" && dir != string(filepath.Separator) {
			out = append(out, dir)
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		out = append(out, dir)
	}
	return out
}()

// redactString removes secrets and the account name from a single string.
func redactString(s string) string {
	if s == "" {
		return s
	}
	for _, home := range homePaths {
		// Windows paths arrive with either separator depending on who wrote
		// them, so both spellings are replaced.
		s = strings.ReplaceAll(s, home, "~")
		s = strings.ReplaceAll(s, strings.ReplaceAll(home, `\`, `/`), "~")
	}
	s = jwtRe.ReplaceAllString(s, redacted)
	s = querySecretRe.ReplaceAllString(s, "${1}"+redacted)
	s = bearerRe.ReplaceAllString(s, redacted)
	s = longSecretRe.ReplaceAllString(s, redacted)
	return s
}

// redactValue walks a value, cleaning every string it finds. Numbers and
// booleans pass through untouched — an altitude is not a secret.
func redactValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return redactString(t)
	case error:
		return redactString(t.Error())
	case []string:
		out := make([]string, len(t))
		for i, s := range t {
			out[i] = redactString(s)
		}
		return out
	case map[string]any:
		return redactMap(t)
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return t
	default:
		// Anything else is rendered and cleaned, so a struct carrying a token
		// in a field cannot slip through as an opaque value.
		return redactString(fmt.Sprint(t))
	}
}

func redactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = redactValue(v)
	}
	return out
}

// beforeSend is the last thing to touch an event. Everything free-form on it
// is cleaned, and the counter totals are attached so a report says what the
// application had been doing, not only what finally broke.
func beforeSend(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	event.Message = redactString(event.Message)
	event.ServerName = "desktop"

	for i := range event.Exception {
		event.Exception[i].Value = redactString(event.Exception[i].Value)
	}
	for i := range event.Breadcrumbs {
		event.Breadcrumbs[i].Message = redactString(event.Breadcrumbs[i].Message)
		event.Breadcrumbs[i].Data = redactMap(event.Breadcrumbs[i].Data)
	}
	for name, ctx := range event.Contexts {
		event.Contexts[name] = sentry.Context(redactMap(ctx))
	}
	for k, v := range event.Tags {
		event.Tags[k] = redactString(v)
	}

	// A desktop client has no inbound request, and anything Sentry inferred
	// for one would only be the local machine.
	event.Request = nil

	// The user is whatever SetPilot recorded: an opaque network identifier and
	// nothing else. Drop anything the SDK inferred on its own.
	event.User = sentry.User{ID: event.User.ID}

	totals := Counters()
	latest := Gauges()
	if len(totals) > 0 || len(latest) > 0 {
		asAny := make(map[string]any, len(totals)+len(latest))
		for k, v := range totals {
			asAny[k] = v
		}
		for k, v := range latest {
			asAny[k] = v
		}
		if event.Contexts == nil {
			event.Contexts = map[string]sentry.Context{}
		}
		event.Contexts["counters"] = sentry.Context(asAny)
	}

	return event
}

// beforeBreadcrumb cleans a breadcrumb before it is stored, so a secret is
// never held in memory waiting for an error to carry it out.
func beforeBreadcrumb(crumb *sentry.Breadcrumb, _ *sentry.BreadcrumbHint) *sentry.Breadcrumb {
	crumb.Message = redactString(crumb.Message)
	crumb.Data = redactMap(crumb.Data)
	return crumb
}
