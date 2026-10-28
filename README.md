# atm-identity-api

Internal microservice that looks up Peruvian identity documents (DNI and RUC) for the
other Atmosfera systems. It keeps its own copy in Postgres, so the paid provider
(APIs Peru) is only called the first time a document is requested.

```
your system --GET /v1/persons/74455518--> atm-identity-api --> Postgres (found? return it)
                                                          '--> APIs Peru (not found? fetch, store, return)
```

Production URL: `https://identity.atmosferast.com`

> Project rules for developers and AI agents live in [AGENTS.md](AGENTS.md).

---

## 1. How many APIs are involved?

**This service exposes 3 endpoints** (2 of them need an API key):

| Method | Path | Needs key | Purpose |
|---|---|---|---|
| GET | `/health` | no | Is the service up? |
| GET | `/v1/persons/{dni}` | yes | Names of a person (DNI, 8 digits) |
| GET | `/v1/companies/{ruc}` | yes | Legal name, trade name and address of a company (RUC, 11 digits) |

**It uses 1 external provider, APIs Peru**, through 2 of its endpoints
(`/dni/{dni}` and `/ruc/{ruc}` at `https://dniruc.apisperu.com/api/v1`).
The token for it lives only in the server's `.env` (`APISPERU_TOKEN`); your systems never see it.

The provider's free plan is 1000 queries per month. This service stops at 900
(`APISPERU_MONTHLY_LIMIT`) to leave a margin.

---

## 2. How a system connects

Every consuming system needs two values in its own configuration:

```env
IDENTITY_API_URL=https://identity.atmosferast.com
IDENTITY_API_KEY=atm_...        # created for that system, see section 3
```

Send the key in the `X-API-Key` header on every `/v1` request.

> Tip: if the system runs in a container on the Docker network `atm-network`, it can use
> `http://identity-api:8080` instead of the public URL (faster, no Cloudflare hop).

### Look up a person (DNI)

```bash
curl -H "X-API-Key: $IDENTITY_API_KEY" https://identity.atmosferast.com/v1/persons/74455518
```

```json
{
  "success": true,
  "data": {
    "document_number": "74455518",
    "first_names": "JOEL JOAS",
    "paternal_surname": "MALDONADO",
    "maternal_surname": "FERNANDEZ",
    "full_name": "JOEL JOAS MALDONADO FERNANDEZ",
    "created_at": "2026-10-27T03:22:46.013558Z",
    "updated_at": "2026-10-27T03:22:46.013558Z"
  },
  "meta": { "request_id": "b0d667482db8af92", "source": "database" }
}
```

### Look up a company (RUC)

```bash
curl -H "X-API-Key: $IDENTITY_API_KEY" https://identity.atmosferast.com/v1/companies/20131312955
```

Returns `document_number`, `legal_name`, `trade_name` (may be `null`) and `address` (may be `null`).

### Notes

- `meta.source` is `"database"` (already stored, free) or `"apisperu"` (just fetched, cost one
  provider query). Your system does not need to care.
- `?refresh=true` forces a new provider query, but only if the stored record is older than 24 hours.
  Use it rarely; each one can cost quota.
- Unknown values are `null`, never an empty string.
- The service stores **only** names/surnames (persons) and legal name, trade name and address
  (companies). Anything else (email, phone, birth date...) stays in your own system.

### What your system must handle

Every error has the same shape:

```json
{ "success": false,
  "error": { "code": "DOCUMENT_NOT_FOUND", "message": "..." },
  "meta": { "request_id": "..." } }
```

| HTTP | `error.code` | Meaning | What your system should do |
|---|---|---|---|
| 200 | | OK | Use `data` |
| 400 | `INVALID_DOCUMENT_NUMBER` | DNI not 8 digits / RUC not 11 digits | Fix the input |
| 401 | `UNAUTHORIZED` | Missing or wrong `X-API-Key` | Check the key |
| 403 | `FORBIDDEN` | Key disabled | Ask the owner for a new one |
| 404 | `DOCUMENT_NOT_FOUND` | The document does not exist | Tell the user |
| 503 | `UPSTREAM_QUOTA_EXCEEDED` | Provider quota (or our monthly cap) used up | **Let the user type the data manually** |
| 503 | `UPSTREAM_UNAUTHORIZED` | Our provider token is invalid/expired | Let the user type it; the owner must fix the token |
| 502 / 504 | `UPSTREAM_FAILED` / `UPSTREAM_TIMEOUT` | Provider failed or was slow | Let the user type it; retry later |
| 500 | `INTERNAL_ERROR` | Bug on our side | Let the user type it |

Simple rule: **if the response is not 200 or 404, fall back to manual entry.** The lookup is a
convenience, never a blocker for saving a customer. Use a timeout of about 15 seconds.

### Code examples

TypeScript / Node (NestJS, etc.):

```ts
export async function lookupPerson(dni: string) {
  const res = await fetch(`${process.env.IDENTITY_API_URL}/v1/persons/${dni}`, {
    headers: { 'X-API-Key': process.env.IDENTITY_API_KEY! },
    signal: AbortSignal.timeout(15_000),
  });
  const body = await res.json();
  if (res.ok) return body.data; // { first_names, paternal_surname, maternal_surname, ... }
  if (body.error.code === 'DOCUMENT_NOT_FOUND') return null; // does not exist
  throw new Error(body.error.code); // anything else: let the user type it manually
}
```

Go:

```go
req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
    os.Getenv("IDENTITY_API_URL")+"/v1/persons/"+dni, nil)
req.Header.Set("X-API-Key", os.Getenv("IDENTITY_API_KEY"))
resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
```

---

## 3. Create an API key for a new system

Each system gets **its own key**, so you can see who uses it and revoke one without
affecting the others. The key is shown **once** and is stored hashed: it cannot be recovered.

On the VPS (`ssh atmosfera`):

```bash
docker exec identity-api /app/identity-api create-client-system <code> "<Name>"
```

Example:

```bash
docker exec identity-api /app/identity-api create-client-system hotel "Hotel system"
```

Prints `atm_...`. Copy it right away into that system's `IDENTITY_API_KEY`.

- `<code>`: lowercase letters, digits, `-` or `_`, starting with a letter (`hotel`, `iris`, `nova`). It must be unique.
- To see the registered systems:
  ```bash
  docker exec atm-postgres psql -U postgres -d identity -c "select code, name, is_active, last_used_at from identity.client_systems"
  ```
- To disable a key (the system then gets `403 FORBIDDEN`):
  ```bash
  docker exec atm-postgres psql -U postgres -d identity -c "update identity.client_systems set is_active=false where code='hotel'"
  ```
- To rotate a key: create a new one with a new code (e.g. `hotel-2`), update the system, then disable the old one.
- A key named `postman-test` was created for manual testing. Disable it when you no longer need it.

---

## 4. Check usage and health

```bash
# queries spent this month on APIs Peru
docker exec atm-postgres psql -U postgres -d identity -c "select * from identity.provider_usage"

# container status and logs
docker ps --filter name=identity-api
docker logs identity-api --tail 50
```

Logs never contain document numbers or the provider token. A warning appears at 80% of the monthly
cap and an error when the cap is reached.

---

## 5. Run locally

Requires Go and the shared Postgres container (`atmosfera-postgres`).

```bash
cp .env.example .env            # then fill in DATABASE_URL and APISPERU_TOKEN
# create the role/database once (see the header of scripts/provision-db.sql)
go run ./cmd/server             # applies migrations on start, listens on :8080
```

```bash
go run ./cmd/server create-client-system dev-local "Local dev"   # prints a key
TEST_DATABASE_URL=<DATABASE_URL> go test -race ./...             # tests (DB tests skip without the variable)
```

## 6. Deploy

Push to `production` (flow: `develop` → `main` → `production`). GitHub Actions checks, builds the image,
publishes it to GHCR and deploys it to the VPS over SSH, applying new migrations first.
Roll back by re-running the workflow on an earlier commit. The production `.env` lives only on the
server at `/opt/identity-api/.env`.
