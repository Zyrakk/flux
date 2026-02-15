# Flux Web

Frontend web para Flux (SvelteKit + Tailwind, adapter Node).

## Scripts

```bash
npm install
npm run dev      # desarrollo local
npm run check    # typecheck Svelte/TS
npm run build    # build producción
npm run start    # ejecuta build con node adapter
```

## Variables de entorno

- `API_INTERNAL_URL` (default: `http://localhost:8080`): URL interna del API usada por el proxy de SvelteKit en `/api/*`.
- `PORT` (default: `3000`)
- `HOST` (default: `0.0.0.0`)

## Auth

El frontend guarda el token en `localStorage` y lo envía como `Authorization: Bearer <token>` en cada request. Si no usas token interno, deja el campo vacío en `/login` y protege con Traefik/Caddy.
