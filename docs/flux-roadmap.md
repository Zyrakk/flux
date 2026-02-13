# Flux — Roadmap Completo de Implementación

## Visión del Proyecto

**Flux** es una plataforma self-hosted de inteligencia informativa que transforma cientos de fuentes diarias en un briefing matutino personalizado. Construido en Go, desplegado en k3s (o Docker Compose), alimentado por LLM vía interfaz abstracta (GLM, OpenAI-compatible, Anthropic).

**Principio core:** Leer menos, leer mejor. El sistema hace el scroll por ti.

> 📄 **Documento complementario: `flux-source-catalog.md`** — Catálogo completo de fuentes RSS, subreddits, y AI labs organizados por sección, con URLs, descripciones, y valoraciones señal/ruido.

---

## Decisiones Arquitectónicas Fundamentales

### Stack Tecnológico

| Componente | Tecnología | Justificación |
|---|---|---|
| Backend / Workers | Go | Rendimiento, concurrencia nativa, ecosistema cloud-native |
| Base de datos relacional | PostgreSQL 16 + pgvector | Una sola DB para datos + embeddings, operacionalmente simple |
| Cola de mensajes | NATS JetStream | Ligero, cloud-native, Go-nativo, perfecto para k3s |
| Cache | Redis (Valkey) | Deduplicación rápida, rate limiting, estado efímero |
| Frontend | Svelte/SvelteKit | Ligero, rápido, SSR nativo, ideal para dashboard |
| LLM | Interfaz abstracta (GLM-4.7 por defecto) | Soporta GLM, OpenAI-compatible (Ollama, vLLM), Anthropic |
| Embeddings | all-MiniLM-L6-v2 (local) | Gratuito, ~80MB, corre en CPU sin problema |
| Ingress | Traefik (incluido en k3s) | Ya disponible en tu cluster |
| Almacenamiento | PVC en el DAS de 24TB (Pi 5) | Archivo histórico masivo |

### Principio de Modularidad

Cada componente es un **microservicio independiente** con su propio Deployment en k3s. Se comunican exclusivamente vía NATS (eventos asíncronos) y PostgreSQL (estado compartido). Si el worker de Reddit se rompe, el de HN sigue funcionando. Si GLM está caído, los artículos se encolan y se procesan cuando vuelva. Nada es síncrono salvo la UI leyendo de la DB.

```
┌─────────────────────────────────────────────────────────────┐
│                        INGRESS (Traefik)                    │
│                     flux.zyrak.cloud                 │
└──────────────┬──────────────────────────────┬───────────────┘
               │                              │
       ┌───────▼───────┐              ┌───────▼───────┐
       │   Frontend    │              │   API Server   │
       │   (Svelte)    │◄────────────►│   (Go)         │
       └───────────────┘              └───────┬───────┘
                                              │
                    ┌─────────────────────────┼──────────────────────┐
                    │                         │                      │
             ┌──────▼──────┐          ┌───────▼───────┐     ┌───────▼──────┐
             │ PostgreSQL  │          │     NATS      │     │    Redis     │
             │ + pgvector  │          │  JetStream    │     │  (Valkey)    │
             └──────▲──────┘          └───────┬───────┘     └──────────────┘
                    │                         │
        ┌───────────┼───────────┬─────────────┼────────────┐
        │           │           │             │            │
   ┌────▼───┐ ┌────▼───┐ ┌────▼───┐  ┌──────▼──┐  ┌──────▼──────┐
   │ Worker │ │ Worker │ │ Worker │  │ Worker  │  │  Processor  │
   │  RSS   │ │   HN   │ │ Reddit │  │ GitHub  │  │  (GLM +     │
   │        │ │        │ │        │  │Releases │  │  Embeddings)│
   └────────┘ └────────┘ └────────┘  └─────────┘  └─────────────┘
```

### Abstracción del LLM (Crítico para Open Source)

El backend LLM se abstrae desde el día 1 mediante una interface Go en `internal/llm/`:

```go
type Analyzer interface {
    Classify(ctx context.Context, articles []Article) ([]Classification, error)
    Summarize(ctx context.Context, article Article) (string, error)
    GenerateBriefing(ctx context.Context, articles []SummarizedArticle) (string, error)
}
```

Implementaciones:
- `glm.go` — GLM-4.7 vía API (tu configuración por defecto con plan Coding Lite)
- `openai_compat.go` — Cualquier API compatible con OpenAI (Ollama, vLLM, LiteLLM, Together, Groq)
- `anthropic.go` — API de Anthropic (Claude)

El usuario elige el backend en el `values.yaml` / `docker-compose.yml` / variables de entorno:
```yaml
llm:
  provider: "glm"           # glm | openai_compat | anthropic
  endpoint: "https://open.bigmodel.cn/api/paas/v4"
  model: "glm-4.7"
  apiKey: "<secret>"
```

Esto te protege a ti (si GLM cambia el plan, migras en 5 minutos) y hace el proyecto usable para cualquiera que tenga un Ollama local o una API key de OpenAI.

### Despliegue Dual: Helm + Docker Compose

El proyecto ofrece dos modos de despliegue:

- **Helm chart** (para k3s/Kubernetes): Tu modo principal, con todas las ventajas de k8s (CronJobs nativos, health checks, rolling updates, nodeSelectors para el cluster híbrido).
- **Docker Compose** (para la mayoría de la comunidad self-hosted): Un `docker-compose.yml` en la raíz del repo que levanta todo con un solo `docker compose up -d`. Mismo stack (PostgreSQL, NATS, Redis, workers, API, frontend), sin necesitar Kubernetes.

Ambos comparten las mismas imágenes Docker. La diferencia es solo orquestación. El Docker Compose se mantiene desde la fase 0 en paralelo al Helm chart — no es un "port" tardío, sino un ciudadano de primera clase.

### Rate Limiting Global de Ingesta

Todos los workers comparten un rate limiter centralizado en Redis para evitar baneos:

```
┌──────────────┐     ┌───────────┐     ┌──────────────────┐
│ Worker RSS   │────►│           │     │ Fuente externa   │
│ Worker HN    │────►│   Redis   │────►│ (Reddit, HN,     │
│ Worker Reddit│────►│  Limiter  │     │  blogs, APIs)    │
│ Worker GitHub│────►│           │     │                  │
└──────────────┘     └───────────┘     └──────────────────┘
```

Configuración por fuente en `internal/ratelimit/`:
- **HN API**: 30 req/min (generosa, pero no abusar)
- **Reddit API**: 60 req/min (límite oficial con OAuth)
- **RSS fetch**: 10 req/min global (descarga de artículos completos con `go-readability`)
- **GitHub API**: 5000 req/h (con token personal)
- **NVD API**: 50 req/30s (con API key)

Además, para la descarga de contenido completo de artículos (`go-readability`):
- Delay aleatorio de 1-3s entre requests al mismo dominio
- Respeto de `robots.txt` y `Retry-After` headers
- User-Agent identificativo (`Flux/1.0 +https://github.com/zyrak/flux`)
- Si un dominio devuelve 429 o 403, backoff exponencial y se marca en Redis para no reintentar en 1h

### Estrategia de Coste con GLM

El plan Coding Lite renueva el cap cada 5 horas. La estrategia:

- **CronJob a las 03:00**: Dispara el pipeline de procesamiento.
- **Fase 1 — Filtrado barato (embeddings locales)**: Calcula similaridad coseno contra el perfil del usuario. Descarta ~80% de artículos. Coste GLM: 0.
- **Fase 2 — Clasificación con GLM**: Solo los ~20% supervivientes pasan por GLM para clasificación y descarte de clickbait. Coste: bajo.
- **Fase 3 — Síntesis con GLM**: Los ~10-15 artículos finales se sintetizan en el briefing. Coste: moderado.
- **Resultado**: A las 07:00–08:00 el briefing está listo y el cap de GLM se ha renovado o está a punto.

---

## Fuentes de Ingesta

> 📄 **El catálogo completo de fuentes está en `flux-source-catalog.md`**, con URLs de feeds, descripción de cada fuente, valoración señal/ruido, y subreddits por sección.

### Resumen por Sección

| Sección | Feeds RSS | Subreddits | Artículos/día (pre-filtro) | En briefing |
|---|---|---|---|---|
| 🔒 Cybersecurity | 12 (tl;dr sec, Krebs, THN, BleepingComputer, Schneier, SANS ISC, Troy Hunt, Miessler, Risky Business, TLDR InfoSec, Dark Reading, Red Hat Security) | 4 (r/netsec, r/cybersecurity, r/AskNetsec, r/blueteamsec) | ~80-120 | 5 |
| 💻 Tech | 14 + 10 AI labs (TLDR, Ars, Lobsters, LWN, Go Blog, K8s Blog, OpenShift Blog, stderr.at, OKD, Papers We Love, Ollama, The Verge + Anthropic, OpenAI, xAI, DeepMind, GLM, Kimi feeds) | 8 (r/kubernetes, r/selfhosted, r/homelab, r/LocalLLaMA, r/MachineLearning, r/golang, r/linux, r/openshift) | ~150-250 | 5 |
| 📈 Economy | 9 (Bloomberg Tech, Reuters Business, CNBC Tech, CoinDesk, The Block, Finimize, FT Tech, TLDR Founders, Expansión) | 6 (r/stocks, r/wallstreetbets, r/CryptoCurrency, r/investing, r/economics, r/nvidia) | ~100-180 | 3 |
| 🌍 World | 6 (Reuters Top, BBC World, AP News, El País, The Guardian, Al Jazeera) | 3 (r/worldnews, r/geopolitics, r/europe) | ~80-150 | 2 |
| **Total** | **~51** | **~21** | **~400-700** | **~15** |

### Fuentes Multi-Sección

Hacker News, Reuters, y Ars Technica generan contenido que cruza secciones. No se asignan a una sección fija — el motor de embeddings clasifica cada artículo en la sección más relevante automáticamente.

### Prioridad de Implementación

- **Fase 1 (MVP)**: RSS de todas las secciones + HN API
- **Fase 4**: Reddit API (21 subreddits) + GitHub Releases + AI labs sin RSS (GLM, Kimi vía GitHub API)
- **Fase posterior**: Bluesky, Mastodon, ArXiv, YouTube transcripts

---

## FASE 0 — Scaffolding y Fundamentos
**Duración estimada: 1.5 semanas**
**Objetivo: Tener el esqueleto del proyecto, CI, infraestructura base desplegada en k3s Y Docker Compose, interfaz LLM abstracta, y rate limiter listo.**

### Tareas

#### 0.1 — Estructura del repositorio
```
flux/
├── cmd/
│   ├── api/              # Servidor API HTTP
│   ├── worker-rss/       # Worker de ingesta RSS
│   ├── worker-hn/        # Worker de ingesta HN
│   ├── worker-reddit/    # Worker de ingesta Reddit
│   ├── processor/        # Pipeline de procesamiento (embeddings + GLM)
│   └── briefing-gen/     # Generador de briefings (CronJob)
├── internal/
│   ├── models/           # Structs compartidos (Article, Briefing, Feedback)
│   ├── store/            # Capa de acceso a PostgreSQL
│   ├── queue/            # Abstracción sobre NATS
│   ├── embeddings/       # Cliente para modelo de embeddings local
│   ├── llm/              # Interfaz abstracta + implementaciones (GLM, OpenAI-compat, Anthropic)
│   ├── dedup/            # Lógica de deduplicación (Redis + hashing)
│   └── ratelimit/        # Rate limiter centralizado por fuente (Redis-backed)
├── web/                  # Frontend Svelte
├── deploy/
│   ├── helm/
│   │   └── flux/        # Helm chart principal
│   └── docker/           # Dockerfiles por servicio
├── docker-compose.yml    # Despliegue alternativo sin Kubernetes
├── migrations/           # Migraciones SQL (golang-migrate)
├── scripts/              # Scripts de utilidad
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

#### 0.2 — Helm Chart base
- Namespace dedicado: `flux`
- PostgreSQL (Bitnami chart como dependencia o tu propia instancia existente)
- NATS (chart oficial)
- Redis/Valkey (chart Bitnami)
- ConfigMap para configuración compartida (lista de feeds, subreddits, etc.)
- Secrets para credenciales (API key GLM, Reddit OAuth, etc.)
- IngressRoute de Traefik con TLS

#### 0.3 — Base de datos — Schema inicial
```sql
-- Secciones del briefing (modulares, activables/desactivables)
CREATE TABLE sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,        -- 'cybersecurity', 'tech', 'economy', 'world'
    display_name TEXT NOT NULL,       -- '🔒 Cybersecurity'
    enabled BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    max_briefing_articles INTEGER DEFAULT 5,
    seed_keywords TEXT[],             -- Para cold start del perfil por sección
    config JSONB                      -- Configuración extra
);

-- Artículos ingestados
CREATE TABLE articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type TEXT NOT NULL,        -- 'rss', 'hn', 'reddit', 'github'
    source_id TEXT NOT NULL,          -- ID único en la fuente original
    section_id UUID REFERENCES sections(id), -- Sección asignada por el motor de relevancia
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT,                     -- Texto completo del artículo
    summary TEXT,                     -- TL;DR generado por LLM
    author TEXT,
    published_at TIMESTAMPTZ,
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ,        -- Cuándo lo procesó el LLM
    embedding vector(384),           -- all-MiniLM-L6-v2 output
    relevance_score FLOAT,           -- Score calculado vs perfil de la sección asignada
    categories TEXT[],               -- Tags asignados por LLM
    status TEXT DEFAULT 'pending',   -- pending, processed, briefed, archived
    metadata JSONB,                  -- Datos extra según fuente (HN score, Reddit upvotes...)
    UNIQUE(source_type, source_id)
);

-- Índices
CREATE INDEX idx_articles_status ON articles(status);
CREATE INDEX idx_articles_published ON articles(published_at DESC);
CREATE INDEX idx_articles_section ON articles(section_id);
CREATE INDEX idx_articles_embedding ON articles USING ivfflat (embedding vector_cosine_ops);

-- Briefings generados
CREATE TABLE briefings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generated_at TIMESTAMPTZ DEFAULT NOW(),
    content TEXT NOT NULL,            -- Briefing completo en Markdown (con secciones)
    article_ids UUID[],              -- Artículos incluidos
    metadata JSONB                   -- Stats por sección: artículos procesados, descartados, etc.
);

-- Feedback del usuario (vinculado a sección vía artículo)
CREATE TABLE feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID REFERENCES articles(id),
    action TEXT NOT NULL,             -- 'like', 'dislike', 'save', 'follow_topic'
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Perfil de intereses POR SECCIÓN (cada sección evoluciona independientemente)
CREATE TABLE section_profiles (
    section_id UUID REFERENCES sections(id),
    positive_embedding vector(384),    -- Centroide de likes en esta sección
    negative_embedding vector(384),    -- Centroide de dislikes en esta sección
    like_count INTEGER DEFAULT 0,
    dislike_count INTEGER DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (section_id)
);

-- Fuentes configuradas
CREATE TABLE sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type TEXT NOT NULL,
    name TEXT NOT NULL,
    config JSONB NOT NULL,            -- URL del feed, subreddit name, etc.
    enabled BOOLEAN DEFAULT TRUE,
    last_fetched_at TIMESTAMPTZ,
    error_count INTEGER DEFAULT 0,
    last_error TEXT
);

-- Relación fuentes ↔ secciones (muchos a muchos)
-- Una fuente puede alimentar múltiples secciones (ej: Reuters → Economy + World)
CREATE TABLE source_sections (
    source_id UUID REFERENCES sources(id),
    section_id UUID REFERENCES sections(id),
    PRIMARY KEY (source_id, section_id)
);

-- Seed data: secciones iniciales
INSERT INTO sections (name, display_name, sort_order, max_briefing_articles, seed_keywords) VALUES
('cybersecurity', '🔒 Cybersecurity', 1, 5, ARRAY['CVE vulnerability exploit', 'ransomware malware threat', 'kubernetes security RBAC', 'zero-day attack', 'data breach incident']),
('tech', '💻 Tech', 2, 5, ARRAY['kubernetes container orchestration', 'golang Go programming', 'LLM AI model release', 'self-hosted open source', 'linux kernel development']),
('economy', '📈 Economy', 3, 3, ARRAY['NVIDIA stock earnings semiconductor', 'Bitcoin cryptocurrency market', 'tech stock earnings revenue', 'Federal Reserve interest rates']),
('world', '🌍 World', 4, 2, ARRAY['geopolitical conflict major event', 'climate disaster emergency', 'election government change', 'international treaty sanctions']);
```

#### 0.4 — CI básico
- GitHub Actions: lint (`golangci-lint`), test, build de imágenes Docker
- Multi-arch builds (amd64 + arm64) — necesario para tu cluster híbrido
- Push a un registry (GitHub Container Registry o tu propio Harbor si tienes)

#### 0.5 — Dockerfiles multi-stage
Un Dockerfile por binario en `deploy/docker/`, todos con el mismo patrón:
```dockerfile
FROM golang:1.23-alpine AS builder
# ... build ...
FROM alpine:3.20
# ... binary + ca-certificates + tzdata
```

#### 0.6 — Interfaz abstracta de LLM
Implementar la interface `Analyzer` en `internal/llm/` con las tres implementaciones desde el inicio:
- `glm.go` — tu backend por defecto (GLM-4.7 vía plan Coding Lite)
- `openai_compat.go` — cualquier API compatible con OpenAI (Ollama, vLLM, LiteLLM, Together, Groq, etc.)
- `anthropic.go` — API de Claude
- `factory.go` — factory que instancia la implementación correcta según configuración

Selección por variable de entorno o config:
```yaml
# En values.yaml / docker-compose.yml / .env
LLM_PROVIDER=glm              # glm | openai_compat | anthropic
LLM_ENDPOINT=https://open.bigmodel.cn/api/paas/v4
LLM_MODEL=glm-4.7
LLM_API_KEY=<secret>
```

Esto es fundacional — hacerlo después requiere refactorizar todo el procesamiento. Hacerlo ahora son 2-3 horas extra y evita meses de dolor.

#### 0.7 — Docker Compose base
`docker-compose.yml` en la raíz del repo como ciudadano de primera clase:
```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
  nats:
    image: nats:2-alpine
    command: ["--jetstream"]
  redis:
    image: valkey/valkey:8-alpine
  api:
    build: { context: ., dockerfile: deploy/docker/Dockerfile.api }
    depends_on: [postgres, nats, redis]
  worker-rss:
    build: { context: ., dockerfile: deploy/docker/Dockerfile.worker-rss }
    depends_on: [postgres, nats, redis]
  worker-hn:
    build: { context: ., dockerfile: deploy/docker/Dockerfile.worker-hn }
    depends_on: [postgres, nats, redis]
  frontend:
    build: { context: ., dockerfile: deploy/docker/Dockerfile.frontend }
    ports: ["8080:8080"]
```

Se mantiene en paralelo al Helm chart. Cada vez que se añade un servicio al Helm chart, se añade al Compose. No es un "port" tardío.

#### 0.8 — Rate Limiter centralizado
Implementar `internal/ratelimit/` con un rate limiter Redis-backed configurable por dominio/fuente:
- Patrón token bucket con Redis (`EVALSHA` de script Lua para atomicidad)
- Configuración por fuente: `{"reddit.com": "60/min", "hn.algolia.com": "30/min", "default": "10/min"}`
- Respeto automático de `Retry-After` headers
- Backoff exponencial por dominio tras 429/403
- Delay aleatorio (1-3s jitter) entre requests al mismo dominio para descarga de contenido
- User-Agent identificativo: `Flux/1.0 (+https://github.com/zyrak/flux)`
- Todos los workers importan este paquete — ningún worker hace requests directos sin pasar por el limiter

### Criterio de Fase Completada
- `helm install flux ./deploy/helm/flux` levanta PostgreSQL, NATS, Redis en tu cluster
- `docker compose up -d` levanta lo mismo en cualquier máquina con Docker
- Las tablas se crean automáticamente (Job de migración)
- El IngressRoute responde en `flux.zyrak.cloud` (aunque solo sea un 200 OK)
- `internal/llm/` compila y tiene tests unitarios para las tres implementaciones (al menos con mocks)
- `internal/ratelimit/` compila y tiene tests que verifican el throttling

---

## FASE 1 — Ingesta Básica (RSS + Hacker News)
**Duración estimada: 1.5–2 semanas**
**Objetivo: Artículos de RSS y HN fluyendo a la base de datos, deduplicados.**

### Tareas

#### 1.1 — Worker RSS
- Parser RSS/Atom usando `gofeed` (librería Go madura)
- Lee la lista de feeds desde la tabla `sources`
- Para cada artículo nuevo:
  - Calcula hash SHA-256 de URL normalizada
  - Check en Redis (`SETNX`) para dedup rápido
  - Si es nuevo, publica evento en NATS `articles.new`
  - Inserta en PostgreSQL con status `pending`
- Descarga contenido completo del artículo con `go-readability` (extrae texto limpio del HTML)
- **Todas las requests HTTP pasan por `internal/ratelimit/`** — respeta los límites por dominio, jitter entre requests, y backoff automático ante 429/403
- **Resiliencia**: si un feed falla, incrementa `error_count` en `sources`, aplica backoff exponencial, continúa con el siguiente feed. Un feed roto no afecta a los demás.
- CronJob en k3s: cada 30 minutos

#### 1.2 — Worker Hacker News
- Usa la API oficial de Firebase de HN (`https://hacker-news.firebaseio.com/v0/`)
- Endpoints: `topstories`, `beststories`, `newstories`
- Para cada story con score > 10 (configurable):
  - Obtiene título, URL, score, comentarios
  - Si tiene URL externa, descarga contenido con `go-readability`
  - Si es un "Ask HN" o "Show HN" sin URL, guarda el texto del post
  - Dedup por URL contra Redis + PostgreSQL
  - Publica en NATS `articles.new`
- Guarda metadata HN en el campo JSONB: `{"hn_score": 142, "hn_comments": 87, "hn_id": 12345}`
- CronJob: cada 15 minutos (la API de HN es generosa y sin auth, pero el rate limiter controla igualmente)

#### 1.3 — Servicio de Deduplicación
- No es un servicio separado, es una librería compartida en `internal/dedup/`
- **Nivel 1**: Hash de URL normalizada (quitar tracking params, normalizar www)
- **Nivel 2**: Detección de "misma historia, diferente URL" — esto se hará en Fase 3 con embeddings. Por ahora, solo URL.
- Redis set con TTL de 7 días para dedup rápido
- PostgreSQL UNIQUE constraint como backup definitivo

#### 1.4 — API Server (endpoints básicos)
- `GET /api/articles` — Lista artículos (paginado, filtrable por source, status, fecha)
- `GET /api/articles/:id` — Detalle de un artículo
- `GET /api/sources` — Lista de fuentes configuradas
- `POST /api/sources` — Añadir nueva fuente
- `PATCH /api/sources/:id` — Habilitar/deshabilitar fuente
- Health check: `GET /healthz`
- Framework: `net/http` estándar + `chi` router (ligero, idiomático)

### Criterio de Fase Completada
- Ejecutar manualmente los workers → artículos aparecen en PostgreSQL
- `curl /api/articles` devuelve artículos reales de HN y tus feeds RSS
- Los workers corren como CronJobs en k3s sin intervención
- Si un feed RSS está caído, los demás siguen funcionando
- Si el worker de HN falla, el de RSS no se entera

---

## FASE 2 — Procesamiento Inteligente (Embeddings + GLM)
**Duración estimada: 2 semanas**
**Objetivo: Los artículos se filtran por relevancia y GLM genera resúmenes.**

### Tareas

#### 2.1 — Servicio de Embeddings Local
- Modelo: `all-MiniLM-L6-v2` (384 dimensiones, ~80MB)
- Opción A: Ejecutar con `onnxruntime` en Go (rendimiento nativo)
- Opción B: Microservicio Python ultra-ligero con `sentence-transformers` + FastAPI (más simple de implementar, ~200MB de imagen Docker)
- **Recomendación: Opción B para empezar**, migrar a Go puro si el rendimiento importa
- El servicio escucha eventos NATS `articles.new`, calcula embedding, actualiza la columna `embedding` en PostgreSQL
- Deployment: 1 réplica, resources limitados (256MB RAM, 0.5 CPU)

#### 2.2 — Motor de Relevancia (Scoring por Sección)
- Al tener el embedding de un artículo:
  - **Paso 1 — Asignación de sección**: Calcula similaridad coseno del artículo contra los `seed_keywords` embeddings de cada sección activa. Asigna a la sección con mayor score. Si la fuente pertenece a una sola sección (`source_sections`), asignación directa sin cálculo.
  - **Paso 2 — Scoring dentro de la sección**: Contra el `section_profiles` de esa sección:
    - `positive_score`: similaridad coseno contra `section_profiles.positive_embedding`
    - `negative_score`: similaridad coseno contra `section_profiles.negative_embedding`
    - `relevance_score = positive_score - (negative_score * 0.5) + source_boost`
    - `source_boost`: bonus configurable por fuente (ej: tl;dr sec = +0.1, Reddit genérico = 0)
- **Perfil inicial (cold start)**: Cada sección se inicializa con embeddings de sus `seed_keywords` definidos en la tabla `sections`. Cybersecurity arranca con "CVE vulnerability", Tech con "kubernetes golang", Economy con "NVIDIA stock crypto", etc. Esto da un filtrado decente desde el día 1 en las 4 secciones.
- Artículos con `relevance_score < threshold` se marcan como `archived` directamente (no pasan por LLM, ahorro de tokens)
- El threshold es **por sección** y se ajusta según volumen: si quedan >50 artículos en una sección, sube; si quedan <5, baja

#### 2.3 — Pipeline LLM (vía interfaz abstracta)
- Usa `internal/llm.Analyzer` — tu configuración por defecto apunta a GLM-4.7, pero cualquier backend funciona
- **CronJob diario a las 03:00**
- Recoge todos los artículos con status `pending` + `relevance_score >= threshold`
- **Paso 1 — Clasificación** (un solo prompt con batch de títulos + primeros párrafos):
  ```
  Clasifica estos artículos. Para cada uno, responde con:
  - relevant: true/false
  - section: una de [cybersecurity, tech, economy, world] (confirma o corrige la sección asignada)
  - clickbait: true/false
  - reason: una frase explicando por qué es o no relevante

  Artículos:
  1. [título] - [sección pre-asignada] - [primer párrafo truncado a 200 chars]
  2. ...
  ```
- **Paso 2 — Resumen** (solo para artículos marcados como `relevant: true`):
  ```
  Resume este artículo en 2-3 frases. Si es una vulnerabilidad, incluye severidad
  y si hay parche. Si es código/herramienta, explica qué hace y por qué importa.
  Si hay datos concretos (benchmarks, cifras), inclúyelos.
  Si es una noticia financiera, incluye cifras clave y tendencia.

  [contenido completo del artículo]
  ```
- **Paso 3 — Síntesis del briefing por secciones** (prompt final):
  ```
  Genera un briefing matutino organizado en las siguientes secciones.
  Para cada sección, destaca el artículo más importante primero.
  Si hay artículos relacionados entre secciones, conéctalos explícitamente.
  Formato: Markdown. Tono: directo, técnico, sin relleno.

  ## 🔒 Cybersecurity (máx 5 artículos)
  [artículos de cybersecurity con sus resúmenes]

  ## 💻 Tech (máx 5 artículos)
  [artículos de tech con sus resúmenes]

  ## 📈 Economy (máx 3 artículos)
  [artículos de economy con sus resúmenes]

  ## 🌍 World (máx 2 artículos)
  [artículos de world con sus resúmenes]
  ```
- El briefing generado se guarda en la tabla `briefings`
- Todos los artículos procesados se marcan como `processed` o `briefed`

#### 2.4 — Gestión de Errores con GLM
- Si GLM está caído o el rate limit está agotado:
  - Los artículos se quedan en `pending`
  - Se reintenta en el siguiente ciclo de 5h
  - El briefing se genera con lo que haya disponible
  - La UI muestra "Briefing parcial — X artículos pendientes de procesamiento"
- Timeouts generosos (120s por request) — GLM puede ser lento en batch
- Logging detallado de tokens consumidos por briefing para monitorizar uso

### Criterio de Fase Completada
- Los artículos tienen embeddings y relevance_score calculados
- El CronJob de las 03:00 genera un briefing real en Markdown
- Puedes leer el briefing en `GET /api/briefings/latest`
- Si GLM falla, el sistema no se rompe — los artículos esperan

---

## FASE 3 — Frontend y Experiencia de Usuario
**Duración estimada: 2 semanas**
**Objetivo: Web UI funcional donde lees el briefing, exploras artículos, y das feedback.**

### Tareas

#### 3.1 — UI del Briefing Matutino (Página principal)
- Dashboard que muestra:
  - **Briefing del día organizado por secciones** (tabs o acordeones: 🔒 Cyber | 💻 Tech | 📈 Economy | 🌍 World)
  - **Estadísticas por sección**: "Cyber: 87 procesados → 5 en briefing | Tech: 192 → 5 | Economy: 134 → 3 | World: 95 → 2"
  - **Hora de generación** y estado ("Completo" / "Parcial — 5 artículos pendientes")
- Cada artículo mencionado en el briefing tiene:
  - Enlace al original
  - Botones de **👍 Like / 👎 Dislike** (el feedback se vincula a la sección del artículo automáticamente)
  - Botón de **🔖 Guardar**
  - Badge de sección con color
  - Fuente de origen (HN, RSS, Reddit) con icono

#### 3.2 — Feed de Artículos
- Página secundaria: lista cronológica de todos los artículos ingestados
- Filtros: por sección, fuente, rango de fechas, solo "liked"
- Cada artículo muestra: título, fuente, sección (badge color), fecha, relevance_score, resumen si existe
- Paginación infinite scroll o botón "cargar más"

#### 3.3 — Sistema de Feedback (por Sección)
- `POST /api/feedback` — registra like/dislike/save, vinculado a la sección del artículo
- Cuando hay nuevo feedback, se recalcula el perfil **de la sección correspondiente**:
  - `section_profiles.positive_embedding` = promedio de embeddings de artículos con like **en esa sección**
  - `section_profiles.negative_embedding` = promedio de embeddings de artículos con dislike **en esa sección**
  - Se usa media móvil exponencial para dar más peso a feedback reciente
- El recálculo es un Job que corre tras cada feedback (o batched cada hora)
- **La UI muestra el efecto por sección**: "🔒 Cyber: 23 likes, 5 dislikes | 💻 Tech: 31 likes, 8 dislikes"
- **Cada sección evoluciona independientemente**: dar like a noticias de NVIDIA en Economy no afecta al perfil de Cybersecurity

#### 3.4 — Gestión de Fuentes y Secciones
- Página de admin: ver todas las fuentes, su sección(es), estado, último fetch, errores
- Añadir nueva fuente RSS con URL y asignar a sección(es)
- Habilitar/deshabilitar fuentes
- Habilitar/deshabilitar secciones completas
- Crear nueva sección (nombre, icono, seed keywords, max artículos en briefing)
- Reordenar secciones en el briefing
- Ver estadísticas por fuente y por sección: artículos ingestados, % que pasa el filtro

#### 3.5 — Diseño y UX
- Mobile-first (lo leerás desde el móvil por la mañana)
- Tema oscuro por defecto (es una herramienta de mañana temprana)
- Minimalista — la información es el protagonista, no la UI
- PWA básico: installable, funciona offline para leer el último briefing cacheado

### Criterio de Fase Completada
- Abres `flux.zyrak.cloud` por la mañana y lees el briefing del día organizado en 4 secciones
- Puedes dar like/dislike a artículos y el feedback evoluciona por sección
- Puedes navegar entre secciones (tabs/acordeones) y ver el feed filtrado por sección
- Puedes añadir/quitar fuentes RSS y asignarlas a secciones desde la UI
- Puedes desactivar una sección completa si no te interesa ese día
- **Esto es el MVP funcional — a partir de aquí, TODO es mejora incremental**

---

## FASE 4 — Más Fuentes y Deduplicación Inteligente
**Duración estimada: 1.5 semanas**
**Objetivo: Reddit como fuente, GitHub Releases, y dedup semántica.**

### Tareas

#### 4.1 — Worker Reddit
- OAuth app (tipo "script", gratis para uso personal)
- Consulta `.json` de cada subreddit configurado (ej: `reddit.com/r/netsec/hot.json`)
- Para posts con score > threshold (configurable por subreddit):
  - Si es link post: descarga artículo externo con `go-readability`
  - Si es self post: guarda el texto del post
  - Guarda metadata: `{"reddit_score": 234, "reddit_comments": 45, "subreddit": "netsec"}`
- Respeta rate limits de Reddit (60 req/min con OAuth)
- CronJob: cada 30 minutos

#### 4.2 — Worker GitHub Releases
- El usuario configura repos a seguir (ej: `kubernetes/kubernetes`, `traefik/traefik`)
- Usa la API de GitHub (con token personal, 5000 req/h)
- Monitoriza nuevas releases: tag, nombre, release notes
- Guarda el changelog/release notes como contenido del artículo
- CronJob: cada 1 hora

#### 4.3 — Deduplicación Semántica
- Problema: la misma noticia aparece en HN, Reddit, y 3 feeds RSS con URLs diferentes
- Solución: tras calcular embedding, busca artículos de las últimas 48h con similaridad coseno > 0.85
- Si hay match, agrupa como "cluster" — guarda `cluster_id` en metadata
- En el briefing, se presentan como uno solo: "Reportado por: HN (142 pts), Reddit r/netsec (89 pts), BleepingComputer"
- Esto mejora drásticamente la calidad del briefing — en vez de ver la misma noticia 4 veces, ves una síntesis con múltiples perspectivas

### Criterio de Fase Completada
- Reddit y GitHub Releases funcionan como fuentes
- Artículos duplicados se agrupan automáticamente
- El briefing muestra "visto en X fuentes" para noticias multi-fuente

---

## FASE 5 — Búsqueda Semántica y Archivo Histórico
**Duración estimada: 1.5 semanas**
**Objetivo: Poder preguntar "¿Qué se dijo sobre X la semana pasada?" y obtener respuestas.**

### Tareas

#### 5.1 — Endpoint de Búsqueda Semántica
- `POST /api/search` con query en lenguaje natural
- Pipeline:
  1. Calcula embedding de la query
  2. Busca los 20 artículos más similares en pgvector (`ORDER BY embedding <=> query_embedding LIMIT 20`)
  3. Filtra por rango de fechas si se especifica
  4. Devuelve resultados con score de similaridad

#### 5.2 — Búsqueda Conversacional (RAG básico)
- `POST /api/ask` con pregunta en lenguaje natural
- Pipeline:
  1. Búsqueda semántica → top 10 artículos relevantes
  2. Prompt a GLM: "Basándote SOLO en estos artículos, responde la pregunta: [pregunta]. Cita tus fuentes."
  3. Devuelve respuesta + artículos fuente
- **Limitación clara**: solo responde sobre artículos que el sistema ha ingestado. No inventa.

#### 5.3 — UI de Búsqueda
- Barra de búsqueda en el header, siempre accesible
- Resultados en tiempo real (embeddings locales = rápido)
- Modo "pregunta" vs modo "buscar artículos"
- El RAG solo se dispara cuando se escribe una pregunta (detectado por `?` o phrasing interrogativo)

#### 5.4 — Archivo y Almacenamiento a Largo Plazo
- Los artículos >30 días se mueven de la tabla principal a `articles_archive` (misma estructura)
- Los embeddings se mantienen — la búsqueda semántica funciona sobre todo el archivo
- El contenido completo (HTML original) se comprime y almacena en el PVC de 24TB
- Stats en la UI: "Tu archivo: 12,847 artículos desde [fecha], ocupando 2.3GB"

### Criterio de Fase Completada
- Puedes buscar "vulnerabilidad kubernetes RBAC" y encontrar artículos relevantes de hace semanas
- Puedes preguntar "¿Qué pasó con la filtración de GLM 5?" y obtener una respuesta basada en tus artículos
- El archivo histórico funciona y crece sin problemas en el DAS

---

## FASE 6 — Story Threading ("Seguir un Tema")
**Duración estimada: 2 semanas**
**Objetivo: Seguir la evolución de una historia a lo largo de días/semanas.**

### Tareas

#### 6.1 — Modelo de Stories
```sql
CREATE TABLE stories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,              -- "Filtración de GLM 5"
    summary TEXT,                     -- Resumen evolutivo generado por GLM
    embedding vector(384),            -- Embedding del tema
    status TEXT DEFAULT 'active',     -- active, stale, closed
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_updated_at TIMESTAMPTZ,
    article_count INTEGER DEFAULT 0
);

CREATE TABLE story_articles (
    story_id UUID REFERENCES stories(id),
    article_id UUID REFERENCES articles(id),
    added_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (story_id, article_id)
);
```

#### 6.2 — Creación Manual de Stories
- Botón "Seguir este tema" en cualquier artículo
- Se crea una story con el embedding del artículo como semilla
- Los nuevos artículos que ingresan se comparan contra stories activas
- Si similaridad > 0.75, se añaden automáticamente a la story

#### 6.3 — Detección Automática de Stories
- Clustering de artículos recientes (últimas 48h) por similaridad de embeddings
- Si un cluster tiene >3 artículos de >2 fuentes diferentes, es candidato a story
- GLM genera un título y resumen para la story detectada
- Se sugiere al usuario: "Posible tema emergente: [título]. ¿Seguir?"

#### 6.4 — UI de Stories
- Página dedicada: lista de stories activas
- Cada story muestra: título, resumen actualizado, timeline de artículos, fuentes involucradas
- El briefing matutino incluye sección "Actualizaciones en tus temas seguidos"

#### 6.5 — Lifecycle de Stories
- Stories sin artículos nuevos en 7 días → status `stale` (se dejan de monitorizar activamente)
- Stories sin artículos en 30 días → status `closed`
- El usuario puede reactivar una story cerrada manualmente

### Criterio de Fase Completada
- Puedes darle "Seguir" a un artículo sobre GLM-5 y al día siguiente aparecen automáticamente los benchmarks filtrados
- El sistema detecta temas emergentes y te los sugiere
- El briefing matutino tiene una sección de "tus temas" con actualizaciones

---

## FASE 7 — Threat Intelligence Integration
**Duración estimada: 2 semanas**
**Objetivo: Convertir Briefing en ThreatBrief — CVEs y advisories matcheados contra tu infra.**

### Tareas

#### 7.1 — Worker NVD/CVE
- API NVD 2.0 (gratuita con API key, 50 req/30s con key)
- Ingesta de nuevos CVEs publicados en las últimas 24h
- Parsea: ID, descripción, CVSS score, CPE affected, referencias
- Guarda como artículos con `source_type = 'nvd'`

#### 7.2 — Inventario de Infraestructura
```sql
CREATE TABLE infrastructure (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_type TEXT NOT NULL,     -- 'container_image', 'helm_release', 'go_dependency'
    name TEXT NOT NULL,               -- 'nginx', 'traefik', 'k3s'
    version TEXT,                     -- '1.25.3'
    namespace TEXT,                   -- k8s namespace
    last_seen TIMESTAMPTZ,
    metadata JSONB                    -- Extra info según tipo
);
```
- **Scanner CronJob**: consulta la API de Kubernetes de tu cluster y rellena esta tabla automáticamente
  - Imágenes de contenedores en uso (con versiones)
  - Helm releases instalados
  - Versión de k3s
- Match automático: cuando entra un CVE que afecta a `nginx 1.24.x` y tú tienes `nginx:1.24.3`, se marca como `high_priority`

#### 7.3 — Briefing de Seguridad
- El briefing matutino gana una sección nueva: "🔒 Seguridad de tu infraestructura"
- CVEs que afectan directamente a tu stack, ordenados por CVSS
- Advisories de vendors relevantes (Red Hat, Kubernetes, etc.)
- GLM genera recomendaciones de acción: "Actualiza nginx a 1.25.4 que parchea CVE-XXXX"

### Criterio de Fase Completada
- El sistema conoce qué software corres en tu cluster
- Los CVEs relevantes para tu infra aparecen destacados en el briefing
- Recibes alertas priorizadas cuando hay CVEs críticos que te afectan

---

## FASE 8 — Notificaciones y Distribución
**Duración estimada: 1 semana**
**Objetivo: Que el briefing te llegue, no que tengas que ir a buscarlo.**

### Tareas

#### 8.1 — Notificaciones por Telegram
- Bot de Telegram que envía el briefing matutino en formato Markdown
- Alertas urgentes para CVEs de severidad Critical que afecten a tu infra
- Comandos básicos: `/briefing` (último briefing), `/search [query]`, `/sources` (estado de fuentes)

#### 8.2 — Email Digest (opcional)
- SMTP con template HTML limpio
- Resumen del briefing + links al dashboard para detalle

#### 8.3 — PWA Push Notifications
- Web Push API para notificaciones en el navegador/móvil
- Solo para alertas de seguridad urgentes — el briefing normal se lee en la web

### Criterio de Fase Completada
- A las 07:00 recibes un mensaje de Telegram con tu briefing
- Si hay un CVE crítico que te afecta, recibes alerta inmediata

---

## FASE 9 — Pulido, Helm Chart Público y Documentación
**Duración estimada: 1.5 semanas**
**Objetivo: Que cualquiera pueda desplegar Briefing en su propio cluster.**

### Tareas

#### 9.1 — Helm Chart Producción
- `values.yaml` bien documentado con todos los valores configurables
- Soporte para bases de datos externas (si el usuario ya tiene PostgreSQL)
- Resource limits y requests ajustados para homelab (bajo consumo por defecto)
- Soporte multi-arch (amd64 + arm64) en todas las imágenes
- Health checks, readiness probes, liveness probes en todos los deployments
- PodDisruptionBudgets donde tenga sentido

#### 9.2 — Onboarding / Setup Wizard
- Primera vez que accedes a la web: wizard de configuración
  - Paso 1: Elegir proveedor LLM (GLM / OpenAI-compatible / Anthropic) y configurar API key
  - Paso 2: Activar/desactivar secciones (🔒 Cyber, 💻 Tech, 📈 Economy, 🌍 World) y opcionalmente crear nuevas
  - Paso 3: Seleccionar fuentes de un catálogo predefinido por sección (con presets recomendados)
  - Paso 4: Configurar horario del briefing
  - Paso 5 (opcional): Conectar Telegram para notificaciones

#### 9.3 — Documentación
- README.md completo con screenshots
- Guía de instalación (Helm + Docker Compose como alternativa)
- Guía de configuración de fuentes
- Guía de desarrollo para contribuidores
- Arquitectura documentada con diagramas

#### 9.4 — Observabilidad
- Métricas Prometheus expuestas por cada servicio
- Dashboard Grafana preconfigurado (como ConfigMap en Helm)
  - Artículos ingestados/hora por fuente
  - Tasa de relevancia (% que pasa el filtro)
  - Uso de GLM (tokens estimados por briefing)
  - Estado de salud de las fuentes
  - Latencia del pipeline

### Criterio de Fase Completada
- `helm install flux oci://ghcr.io/zyrak/flux` funciona en un cluster limpio
- `docker compose up -d` funciona en cualquier máquina con Docker
- Un usuario nuevo puede tener el sistema funcionando en <15 minutos (con cualquiera de los dos métodos)
- El README tiene screenshots y documentación clara
- Tu Grafana muestra el dashboard de Flux junto a tus dashboards existentes

---

## Resumen de Fases

| Fase | Nombre | Duración | Lo que obtienes |
|---|---|---|---|
| 0 | Scaffolding + LLM Abstraction + Rate Limiter | 1.5 semanas | Repo, Helm + Docker Compose, DB, CI, interfaz LLM abstracta, rate limiter |
| 1 | Ingesta RSS + HN | 1.5–2 sem | Artículos fluyendo a la DB con rate limiting |
| 2 | Procesamiento LLM | 2 semanas | Briefing diario generado automáticamente |
| 3 | Frontend + Feedback | 2 semanas | **MVP funcional — usable a diario** |
| 4 | Reddit + GitHub + Dedup | 1.5 sem | Más fuentes, sin duplicados |
| 5 | Búsqueda Semántica | 1.5 sem | "¿Qué pasó con X?" respondido |
| 6 | Story Threading | 2 semanas | Seguir la evolución de temas |
| 7 | Threat Intelligence | 2 semanas | CVEs matcheados contra tu infra |
| 8 | Notificaciones | 1 semana | Telegram + alertas |
| 9 | Producción + Docs | 1.5 sem | Helm chart + Docker Compose público, onboarding |
| **Total** | | **~17 semanas** | **Plataforma completa** |

---

## Notas de Viabilidad y Riesgos

### Riesgo: Cambios en APIs externas
- Reddit ha restringido su API en 2023 pero el tier gratuito con OAuth sigue funcionando para uso personal
- HN usa Firebase y nunca ha cambiado su API en 10+ años
- NVD API 2.0 es un servicio gubernamental estable
- **Mitigación**: Cada worker es independiente. Si Reddit cierra su API mañana, pierdes una fuente pero el sistema sigue vivo.

### Riesgo: Cambios en el proveedor LLM
- GLM puede cambiar el plan Coding Lite, subir precios, o limitar el uso vía API
- **Mitigación**: La interfaz abstracta `internal/llm.Analyzer` permite cambiar de backend en minutos. Si GLM deja de funcionar, configuras Ollama local, OpenAI, o Anthropic sin tocar una línea de lógica de negocio.

### Riesgo: Baneo por exceso de requests
- Reddit, blogs individuales, y APIs pueden banear IPs que hacen demasiadas requests
- `go-readability` descargando contenido completo puede triggear protección anti-bot en algunos sitios
- **Mitigación**: El rate limiter centralizado en `internal/ratelimit/` con Redis controla todas las requests salientes. Jitter aleatorio, respeto de `Retry-After`, backoff exponencial, y User-Agent identificativo reducen el riesgo a mínimos.

### Riesgo: Calidad del filtrado en cold start
- Sin feedback, el sistema depende de las seed keywords para el perfil inicial
- Los primeros 3-5 días el briefing será imperfecto
- **Mitigación**: Hacer el threshold de relevancia más permisivo los primeros 7 días. Mejor mostrar algo de ruido que perder señal.

### Riesgo: Capacidad de GLM en el cap de 5h
- Con el plan Coding Lite, no hay límite de tokens explícito sino un "cap" por ventana de 5h
- Si procesas ~200 artículos con clasificación + ~30 con resumen + 1 briefing completo, es un volumen moderado
- **Mitigación**: El filtrado por embeddings (gratuito, local) reduce drásticamente lo que llega a GLM. Monitoriza el uso y ajusta el threshold si te acercas al límite.

### Riesgo: Complejidad acumulada
- 17 semanas es optimista si trabajas solo en tiempo libre — cuenta con 5-6 meses realistas para el MVP (fases 0-3) y 9-12 meses para el proyecto completo
- **Mitigación**: Las fases 0-3 son el MVP. Si solo llegas ahí, ya tienes un producto útil. Todo lo demás es mejora incremental. No intentes construir la fase 7 sin haber usado la fase 3 durante al menos dos semanas.
