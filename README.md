# Flux

Self-hosted intelligent news briefing platform.

Read less, read better.

Flux ingests high-volume tech news streams (RSS, Hacker News), processes them with local embeddings and LLM summarization, and serves a daily briefing with feedback-driven personalization.

## Table Of Contents

- [What Flux Delivers](#what-flux-delivers)
- [Architecture](#architecture)
- [Stack](#stack)
- [Repository Layout](#repository-layout)
- [Quick Start (Docker Compose)](#quick-start-docker-compose)
- [Frontend Routes](#frontend-routes)
- [API Reference](#api-reference)
- [Feedback And Section Profile Recalculation](#feedback-and-section-profile-recalculation)
- [Authentication Modes](#authentication-modes)
- [PWA / Offline Support](#pwa--offline-support)
- [Configuration](#configuration)
- [Deploy To k3s With Helm](#deploy-to-k3s-with-helm)
- [Phase 3 Verification Checklist](#phase-3-verification-checklist)
- [Troubleshooting](#troubleshooting)
- [Development Commands](#development-commands)
- [Roadmap And Docs](#roadmap-and-docs)
- [License](#license)

## What Flux Delivers

MVP (Phase 3) includes:

- Daily briefing UI (`/`) with section tabs and markdown rendering.
- Full feed (`/feed`) with filters, infinite scroll, and feedback actions.
- Admin panel (`/admin/sources`) for sources and section management.
- Feedback loop (`like`, `dislike`, `save`) with section profile recalculation.
- Personal auth (single-user bearer token) with optional reverse-proxy auth.
- PWA basics (installable manifest + service worker caching latest briefing).
- Docker Compose and Helm deployment paths.

## Architecture

```text
                         +-----------------------+
                         |      Frontend         |
                         |   SvelteKit (SSR)     |
                         +-----------+-----------+
                                     |
                      /api proxy     |
                                     v
+----------+      +------------------+------------------+      +--------+
| workers  +----->|                API                  +----->| Redis  |
| RSS/HN/  | NATS |      Go (chi, auth, REST)          |      +--------+
| Reddit/  |      |                                    |
| GitHub   |      |                                    |
+-----+----+      +------------------+------------------+
      |                              |
      |                              v
      |                    +---------+---------+
      |                    | PostgreSQL +      |
      |                    | pgvector          |
      |                    +---------+---------+
      |                              ^
      v                              |
+-----+------------------------------+------+
|               Processor                   |
| embeddings + relevance + profile recalc   |
+-------------------------------------------+

Additional services:
- NATS JetStream (event bus)
- Embeddings service (all-MiniLM-L6-v2, 384 dims)
- Briefing generator (LLM classify/summarize/compose)
```

Pipeline at a high level:

1. Workers ingest articles and publish `articles.new`.
2. Processor embeds/classifies, assigns section, stores relevance/status.
3. Briefing generator produces daily markdown briefing per active section.
4. Frontend displays briefing/feed/admin and sends feedback.
5. Feedback updates section profiles (immediate or hourly, configurable).

## Stack

| Layer | Technology |
| --- | --- |
| Backend API | Go + chi |
| Workers / Processor | Go |
| DB | PostgreSQL 16 + pgvector |
| Queue | NATS JetStream |
| Cache / Rate Limit | Redis (Valkey-compatible) |
| Embeddings | all-MiniLM-L6-v2 (local service) |
| Frontend | SvelteKit (adapter-node) + Tailwind CSS |
| Deploy | Docker Compose, Helm (k3s/Traefik) |

## Repository Layout

```text
cmd/
  api/            # REST API
  worker-rss/     # RSS ingestion
  worker-hn/      # Hacker News ingestion
  worker-reddit/  # Reddit ingestion (OAuth script flow)
  worker-github/  # GitHub releases ingestion
  processor/      # embeddings + relevance + section profile hourly loop
  briefing-gen/   # briefing generation job/daemon
internal/         # domain logic: config, llm, profile, store, queue, etc.
web/              # SvelteKit frontend
migrations/       # SQL schema and seed data
deploy/docker/    # Dockerfiles per service
deploy/helm/flux/ # Helm chart
docs/             # roadmap and source catalog
```

## Quick Start (Docker Compose)

### Prerequisites

- Docker + Docker Compose plugin
- LLM API key (`LLM_API_KEY`) for briefing/summarization

### 1) Configure env

```bash
cp .env.example .env
```

Edit at minimum:

- `LLM_API_KEY`
- `POSTGRES_PASSWORD`
- `AUTH_TOKEN` (optional, recommended for personal deployments)

Security notes:
- Keep secrets only in `.env` (already gitignored).
- Do not put real credentials directly in `docker-compose.yml`.

### 2) Start stack

```bash
docker compose up -d --build
```

Open:

- Frontend: `http://localhost:8080`

Notes:

- The API is intentionally internal in compose. Use the frontend proxy (`/api/...`) from your browser/curl.
- `briefing-gen` is under compose profile `manual` by default.

### 3) Run briefing generator (optional but usually needed)

Run one briefing now:

```bash
docker compose run --rm briefing-gen
```

Or run scheduler container:

```bash
docker compose --profile manual up -d briefing-gen
```

### 4) Observe logs

```bash
docker compose logs -f api processor worker-rss worker-hn worker-reddit worker-github
```

## Frontend Routes

| Route | Purpose |
| --- | --- |
| `/` | Latest briefing page: markdown + per-section tabs + feedback actions |
| `/feed` | Full chronological feed with filters and infinite scroll |
| `/admin/sources` | Source list/toggles, RSS validation, section create/reorder/limits |
| `/login` | Token input page for built-in bearer auth |
| `/api/*` | Frontend server-side proxy to API (`API_INTERNAL_URL`) |

## API Reference

Base path: `/api` (protected by bearer auth only if `AUTH_TOKEN` is set).

Public health endpoint on API container: `/healthz` (not routed via frontend `/api` proxy).

### Articles

- `GET /api/articles`
  - Query params:
    - `page`, `per_page` (max `100`)
    - `section` or `sections` (comma-separated)
    - `source_type`, `source_ref`
    - `status` (`pending|processed|briefed|archived`)
    - `from`, `to` (ISO-8601 date or RFC3339)
    - `liked_only` (`true|false`)
- `GET /api/articles/{id}`

### Sources

- `GET /api/sources`
- `POST /api/sources`
- `PATCH /api/sources/{id}`
- `POST /api/sources/validate-rss`

### Sections

- `GET /api/sections`
- `POST /api/sections`
- `PATCH /api/sections/{id}`
- `POST /api/sections/reorder`

### Briefings

- `GET /api/briefings/latest`
- `GET /api/briefings`
- `GET /api/briefings/{id}`

### Feedback

- `POST /api/feedback`
  - Body: `{"article_id":"uuid","action":"like|dislike|save"}`
- `GET /api/feedback/stats`
- `DELETE /api/feedback/{id}`

### Example requests via frontend proxy

```bash
TOKEN="your-token-if-enabled"

curl -H "Authorization: Bearer ${TOKEN}" \
  http://localhost:8080/api/briefings/latest

curl -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:8080/api/articles?page=1&per_page=20&sections=cybersecurity,tech&liked_only=true"

curl -X POST -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{"article_id":"<uuid>","action":"like"}' \
  http://localhost:8080/api/feedback
```

## Feedback And Section Profile Recalculation

Section profiles are stored in `section_profiles` with:

- `positive_embedding` (likes)
- `negative_embedding` (dislikes)
- `like_count`, `dislike_count`, `updated_at`

Recalculation behavior:

- On `like` or `dislike`, recalculation runs immediately when `PROFILE_RECALC_TRIGGER=immediate`.
- With `PROFILE_RECALC_TRIGGER=hourly`, processor recalculates all sections every `PROFILE_RECALC_EVERY` (and once at startup).
- `save` does not trigger profile recomputation.

Embedding update strategy:

- Recent feedback centroid is blended with historical profile using EMA weights:
  - `0.7` recent
  - `0.3` historical
- If a section has no likes yet, positive profile falls back to seed keyword embedding.
- If a section has no dislikes yet, negative profile remains unchanged.

## Authentication Modes

Flux supports two simple personal-deployment patterns:

### 1) Built-in token auth

- Set `AUTH_TOKEN` in environment.
- API middleware enforces `Authorization: Bearer <token>`.
- Frontend `/login` stores token in `localStorage` and adds it to all requests.

### 2) Reverse-proxy auth

- Leave `AUTH_TOKEN` empty.
- Enforce auth at Traefik/Caddy (BasicAuth or forward auth).
- Frontend can run with empty local token.

## PWA / Offline Support

Frontend includes:

- `web/static/manifest.webmanifest` (`name: Flux`, dark theme colors)
- Service worker (`web/src/service-worker.ts`) that:
  - Caches static assets
  - Uses network-first strategy for `/api/briefings/latest`
  - Falls back to cached latest briefing when offline

Installable on Chrome/Safari mobile as a standalone app.

## Configuration

Main env vars (`.env.example`):

| Area | Variables |
| --- | --- |
| Core | `DATABASE_URL`, `NATS_URL`, `REDIS_URL` |
| LLM | `LLM_PROVIDER`, `LLM_ENDPOINT`, `LLM_MODEL`, `LLM_API_KEY` |
| Embeddings | `EMBEDDINGS_URL` |
| Relevance | `RELEVANCE_THRESHOLD_DEFAULT`, `RELEVANCE_THRESHOLD_MIN`, `RELEVANCE_THRESHOLD_MAX`, `RELEVANCE_THRESHOLD_STEP`, `SOURCE_BOOSTS` |
| Briefing | `BRIEFING_SCHEDULE` |
| API/Auth | `API_PORT`, `AUTH_TOKEN`, `LOG_LEVEL` |
| Profile Recalc | `PROFILE_RECALC_TRIGGER`, `PROFILE_RECALC_EVERY` |
| Workers | `WORKER_MODE_RSS`, `WORKER_MODE_HN`, `WORKER_MODE_REDDIT`, `WORKER_MODE_GITHUB`, `HN_MIN_SCORE`, `RATE_LIMITS`, `USER_AGENT`, `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `REDDIT_USERNAME`, `REDDIT_PASSWORD`, `GITHUB_TOKEN` |
| Frontend | `API_INTERNAL_URL` |

## Deploy To k3s With Helm

### 1) Image strategy (default: GHCR)

Default chart values use GHCR-hosted Flux images and pull them directly from each node:

- `api.image.repository: ghcr.io/zyrakk/flux-api` (same pattern for other Flux services)
- `global.imagePullPolicy: IfNotPresent`
- `global.imageRegistry: ""` (kept empty so dependency charts keep their own upstream registries)

CI publishes multi-arch images (`linux/amd64`, `linux/arm64`) to GHCR for all Flux services.

For local-only image testing (without registry), use the override file:

```bash
helm upgrade --install flux ./deploy/helm/flux -n flux \
  -f deploy/helm/flux/values.local-images.yaml
```

### Secret management (recommended)

- Copy `deploy/helm/flux/values.secrets.example.yaml` to `deploy/helm/flux/values.secrets.local.yaml` (gitignored pattern recommended) and fill real values.
- Or create a Kubernetes secret manually and set:
  - `secrets.create=false`
  - `secrets.existingSecret=<your-secret-name>`

### 2) Install / upgrade

```bash
helm upgrade --install flux ./deploy/helm/flux -n flux --create-namespace
```

If your GHCR images are private, create a pull secret and set:

- `global.imagePullSecrets[0]=<secret-name>`

### 3) Ingress routing split

Helm ingress template routes:

- `PathPrefix(/api)` -> API service
- `/` -> Frontend service

This is required so Traefik serves web UI at root while keeping API under `/api`.

### 4) TLS options

The chart expects TLS secret name from values:

- `ingress.tls.secretName` (default `flux-tls`)

Choose one:

1. cert-manager managed certificate:
   - Set `ingress.tls.certManager.enabled=true`
   - Ensure issuer exists (`letsencrypt-prod` by default)
2. Existing secret:
   - Create manually:

```bash
kubectl -n flux create secret tls flux-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key
```

If `flux-tls` is missing and cert-manager is not provisioning it, HTTPS ingress will fail.

## Phase 3 Verification Checklist

Local checks:

1. Frontend serves HTML:
   - `docker compose up -d`
   - `curl http://localhost:8080`
2. Briefing page (`/`) loads latest briefing and section tabs.
3. Feed (`/feed`) loads, filters apply, infinite scroll works.
4. Feedback actions persist in DB:
   - `docker compose exec postgres psql -U flux -d flux -c "SELECT * FROM feedback ORDER BY created_at DESC LIMIT 5;"`
5. Section profile counts update:
   - `docker compose exec postgres psql -U flux -d flux -c "SELECT s.display_name, sp.like_count, sp.dislike_count, sp.updated_at FROM section_profiles sp JOIN sections s ON sp.section_id = s.id;"`
6. Admin page (`/admin/sources`) can toggle/add source and manage sections.
7. Auth:
   - No token (when required) -> `401`
   - Correct token -> requests succeed

k3s checks:

1. `helm upgrade --install flux ./deploy/helm/flux -n flux`
2. `kubectl get pods -n flux`
3. Verify ingress host serves frontend and `/api` calls work.

## Troubleshooting

### Pods in `ErrImagePull` / `ErrImageNeverPull`

Cause:

- Chart/release still points to local `flux-*:latest` images with `imagePullPolicy: Never`
- Or local-images override is used but images were not imported on the target node

Fix:

- Use default GHCR chart values (`ghcr.io/zyrakk/flux-*`, `IfNotPresent`), or
- Keep local mode and import images to the node(s) that schedule Flux workloads.

### `flux-tls` secret not found

Cause:

- TLS secret expected by ingress, but not created.

Fix:

- Enable cert-manager certificate creation in chart values, or
- Create `flux-tls` manually as shown above.

### API returns `401 unauthorized`

Cause:

- `AUTH_TOKEN` is set and request has missing/invalid bearer token.

Fix:

- Login at `/login` and set correct token, or
- Remove/empty `AUTH_TOKEN` if auth is handled upstream.

### No briefing appears today

Cause:

- No briefing generated yet (or generator not running).

Fix:

- Run `docker compose run --rm briefing-gen`
- Or start scheduler with `docker compose --profile manual up -d briefing-gen`

### Feed is empty

Check:

- Worker logs (`worker-rss`, `worker-hn`, `worker-reddit`, `worker-github`)
- Source enabled flags in `/admin/sources`
- Database connectivity and NATS health

## Development Commands

From repository root:

```bash
make build            # build all Go binaries
make test             # go test -race -count=1 ./...
make lint             # golangci-lint run ./...
make docker-build     # build service images
make compose-up       # docker compose up -d
make compose-down     # docker compose down
make helm-template    # render chart locally
```

Frontend dev loop:

```bash
cd web
npm ci
npm run dev
npm run check
npm run build
```

## Roadmap And Docs

- Roadmap: [`docs/flux-roadmap.md`](docs/flux-roadmap.md)
- Source catalog: [`docs/flux-source-catalog.md`](docs/flux-source-catalog.md)

## License

Apache 2.0
