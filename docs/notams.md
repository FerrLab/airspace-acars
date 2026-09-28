# Company NOTAMs in ACARS

The NOTAMs tab reads notices from the selected tenant using the current pilot
Bearer token. It does not require a simulator connection or an active booking.
Local mode makes no NOTAM requests. Switching company discards the previous
company's notices and pending UI responses.

## API contract

- List: `GET /api/v1/company-notams?filter[is_active]=1&sort=created_at&sort_dir=desc&per_page=25&page=1`
- Detail: `GET /api/v1/company-notams/{id}`
- Pagination: Laravel `data` + `meta.current_page` / `meta.last_page`; flat
  `current_page` / `last_page` is also accepted.
- IDs may be JSON numbers or strings. Notices use `title`, `created_at`, and
  `content` (with `body` and `description` accepted as text fallbacks). The
  detail response may be a resource under `data` or a direct object.
- Rich-text HTML is displayed as plain text with paragraph breaks. Scripts,
  embedded content and remote images are not rendered.

The manual documents the routes but not the full notice response schema.
The field mapping above still needs verification against a real tenant.

## Authentication limitation

The [Private API v1](https://airspace.ferrlab.com/manual/es/api/private-api-v1)
documents an integration API key. Acceptance of the ACARS pilot token is **not
confirmed**. This client attempts read-only access with its existing pilot
session and probes known company and ACARS candidate routes (`/api/v1/company-notams`,
`/api/v2/acars/notams`, `/api/v2/acars/company-notams`). It never embeds an
administrative API key and halts candidate probing immediately on 401/403 or 429.

A 401/403 shows an access message in the tab. It must not trigger ACARS logout:
the HTTP adapter only treats a 401 from `/api/v2/acars/` as session expiration.
A 404/405 indicates unavailable notices, 429 asks the pilot to wait, and network
or malformed-response failures offer a manual refresh. None is presented as an
empty successful list.

If the deployed server rejects pilot tokens, a server change is required to
provide read-only company notices under pilot authentication. The UI and Wails
service are ready, but client code alone cannot grant that access.

## Verification

`go test ./internal/app ./internal/adapters/airspace` and `npm test` in `frontend`
cover parsing, pagination, unavailable access without logout, local mode,
company switching, safe text rendering and translations (en, pt, es, fr).
Regenerate bindings with `wails3 generate bindings -ts` before building.

For a real-tenant smoke test, sign in normally, open NOTAMs, check a notice's
full text and pagination, and refresh. If access is denied, confirm that ACARS
remains signed in. No new credentials are requested by the tab.
