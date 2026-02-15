# Flux

**Self-hosted intelligent news briefing platform.** Read less, read better.

> 🚧 Work in progress — see [roadmap](docs/flux-roadmap.md) for details.

## Quick Start (Docker Compose)

```bash
cp .env.example .env
# Edit .env (LLM_API_KEY and optional AUTH_TOKEN)
docker compose up -d --build
```

Frontend: `http://localhost:8080`

## Authentication Options

1. Built-in token auth (single user):
- Set `AUTH_TOKEN` in `.env`.
- Login in `/login` and the frontend stores the token in `localStorage`.
- Requests are sent with `Authorization: Bearer <token>`.

2. Reverse-proxy auth (alternative):
- Leave `AUTH_TOKEN` empty.
- Enforce auth in Traefik/Caddy (BasicAuth or forward auth).
- Frontend can operate without local token.

## Notes

- Frontend is SvelteKit (node adapter) + Tailwind, dark by default, mobile-first.
- PWA manifest + service worker cache the latest briefing for offline reading.

## License

Apache 2.0
