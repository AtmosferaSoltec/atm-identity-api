# atm-identity-api: project rules

Read this file fully before changing anything. It is the single source of truth for every
AI agent (Claude Code, Codex, Gemini, Antigravity, ...) and for humans. `CLAUDE.md` and
`GEMINI.md` only point here. When a rule changes, change it here.

## 1. What this service is

A small internal microservice that resolves Peruvian identity documents for the other
Atmosfera systems (hotel, iris, nova, ...). It sits in front of the paid third-party API
(APIs Peru) and keeps its own Postgres copy.

```
client system -> GET /v1/persons/{dni}   (or /v1/companies/{ruc})
                    |- found in Postgres?  yes -> return it (source: "database")
                    '- no -> call APIs Peru -> insert into Postgres -> return it (source: "apisperu")
```

Rules of the flow:

- Callers never know or care where data came from. `meta.source` is informational only.
- The database is only a cache of what APIs Peru returns. Nothing else is stored: no
  client-supplied data and no enrichment. Systems keep their own extra customer data.
- A document that APIs Peru cannot find is **not** stored; the API returns `404 DOCUMENT_NOT_FOUND`.
- Two systems asking for the same new document at once must not cause two upstream calls
  or a duplicate row (`INSERT ... ON CONFLICT`, or a per-document lock).
- Stored fields only: persons = `document_number`, `first_names`, `paternal_surname`, `maternal_surname`;
  companies = `document_number`, `legal_name`, `trade_name`, `address`. Do not add more without asking the owner.
- Refresh (`?refresh=true`) calls APIs Peru again and overwrites the stored values.

## 2. Language: everything in English

Code, identifiers, tables, columns, folders, files, commits, comments, docs, log messages,
error messages, API fields: **English only**. No Spanish names anywhere.
Peruvian concepts keep a neutral English name:

| Concept | Name used |
|---|---|
| DNI / RUC | `document_number` (with `document_type` where needed); routes use `persons` / `companies` |
| nombres | `first_names` |
| apellido paterno | `paternal_surname` |
| apellido materno | `maternal_surname` |
| razon social / nombre comercial | `legal_name` / `trade_name` |
| direccion | `address` |

Names are always stored **split** (never a single full-name column). A `full_name` may be
computed in the response, never persisted.
Upstream (Spanish) field names exist only inside the APIs Peru client and are mapped there.

## 3. Stack

Go 1.27, **Gin** (HTTP), **pgx v5** (`pgxpool`), **goose** (SQL migrations in `db/migrations/`),
`log/slog` JSON logs, PostgreSQL 18 (shared container `atmosfera-postgres`, own database
`identity`, own schema `identity`, own role `identity_app`). Same conventions as `atm-orbita-api`.
Do not add an ORM. Add a dependency only when needed and say why.

## 4. Layout

```
cmd/server/            main.go: wiring only, no logic
internal/
  platform/            apiresponse, apierror, config, database, auth, logger
  persons/             one folder per domain: handler, service, repository, model
  companies/
  apisperu/            HTTP client for APIs Peru + mapping of its Spanish fields
db/migrations/         goose files: 00001_identity_<what>.sql (Up and Down)
scripts/               provision-db.sql and other ops scripts
docs/                  extra documentation
```

Handlers parse and validate input, call a service, and render with `apiresponse`. Services hold
the logic and know nothing about Gin. Repositories hold SQL and know nothing about HTTP.

## 5. Response contract (mandatory)

Every response, including 404 and panics, uses one envelope from `internal/platform/apiresponse`.
**Never call `c.JSON` in a handler**; use `apiresponse.OK / Created / Fail`.

Success:

```json
{
  "success": true,
  "data": { "document_number": "74455518", "first_names": "JOHN", "paternal_surname": "DOE",
            "maternal_surname": "ROE", "full_name": "JOHN DOE ROE" },
  "meta": { "request_id": "b1f0...", "source": "database" }
}
```

Error:

```json
{
  "success": false,
  "error": { "code": "INVALID_DOCUMENT_NUMBER", "message": "A DNI must have exactly 8 digits.",
             "details": [ { "field": "document_number", "issue": "must have 8 digits" } ] },
  "meta": { "request_id": "b1f0..." }
}
```

- JSON keys are `snake_case`. Dates are `YYYY-MM-DD`, timestamps RFC 3339 UTC. Unknown values are `null`.
- Error `code`s are stable (UPPER_SNAKE_CASE, defined in `internal/platform/apierror`); add, never rename.
- Return errors as `*apierror.Error`. Anything else becomes `500 INTERNAL_ERROR`; the cause is logged
  with the `request_id` and is **never** sent to the client.

| HTTP | code | When |
|---|---|---|
| 400 | `VALIDATION_FAILED` | malformed body or params (`details` lists fields) |
| 400 | `INVALID_DOCUMENT_NUMBER` | DNI not 8 digits / RUC not 11 digits |
| 401 | `UNAUTHORIZED` | missing or invalid `X-API-Key` |
| 403 | `FORBIDDEN` | key valid but inactive or not allowed |
| 404 | `DOCUMENT_NOT_FOUND` | not in database and not found upstream |
| 429 | `RATE_LIMITED` | too many requests |
| 502 | `UPSTREAM_FAILED` | APIs Peru returned an error |
| 504 | `UPSTREAM_TIMEOUT` | APIs Peru timed out |
| 500 | `INTERNAL_ERROR` | anything unexpected |

## 6. API surface

All under `/v1`, all requiring `X-API-Key` except `/health`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | liveness, no auth |
| GET | `/v1/persons/{document_number}` | resolve a DNI (`?refresh=true` forces upstream) |
| GET | `/v1/companies/{document_number}` | resolve a RUC |

Each consuming system has a row in `client_systems` with a hashed API key (`code` like `hotel`),
so we know who read or wrote what and can revoke one key alone.

## 7. Database rules

- Tables and columns: English, `snake_case`, plural table names. `TIMESTAMPTZ` in UTC. `NULL` over empty strings.
- Natural unique key: `document_number` (`persons`: 8 digits, `companies`: 11 digits).
- Migrations are the only way to change the schema: one file per change, goose `Up` and `Down`,
  never edit an applied migration. The app role has no access to `public`.
- Local DB: container `atmosfera-postgres`; create role/db with `scripts/provision-db.sql`.
  Production uses the same shared container on the VPS.

## 8. Security and privacy

- Secrets only in `.env` (git-ignored). Keep `.env.example` in sync (no real values).
- The service is internal: reachable only on the Docker network, no public port in production.
- Documents and names are personal data (Peru Law 29733): never log a full document number
  (mask: `7445****`), never log request bodies, back up the database encrypted.
- API keys are stored hashed; compare in constant time.

## 9. Working agreements

- Verify before claiming done: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`.
- Tests mock the APIs Peru client; no test may hit the real upstream.
- Integration tests need a migrated Postgres and are skipped without `TEST_DATABASE_URL`
  (locally: the `DATABASE_URL` from `.env`). They only touch document numbers starting with `9999`.
  Run: `TEST_DATABASE_URL=<url> go test -race ./...`
- Run locally: `go run ./cmd/server` (applies migrations on start). Subcommands: `migrate`,
  `create-client-system <code> <name>` (prints the API key once).

- Keep changes small and in the layout above. Do not reopen decisions in this file without asking the owner.
