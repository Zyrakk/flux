# Flux — Catálogo de Fuentes por Sección

## Arquitectura de Secciones

Cada sección es una entidad independiente con sus propias fuentes, perfil de relevancia (embeddings), y configuración. El briefing matutino genera un bloque por sección activa. Las secciones son modulares: se pueden activar, desactivar, o crear nuevas desde la UI sin tocar código.

```
┌─────────────────────────────────────────────────────┐
│                  BRIEFING MATUTINO                   │
├─────────────┬──────────────┬──────────┬─────────────┤
│ 🔒 Cyber    │ 💻 Tech      │ 📈 Economy│ 🌍 World   │
│ 5 noticias  │ 5 noticias   │ 3 noticias│ 2 noticias │
├─────────────┼──────────────┼──────────┼─────────────┤
│ RSS propios │ RSS propios  │RSS propios│ RSS propios │
│ Subreddits  │ Subreddits   │Subreddits│ Subreddits  │
│ HN (filtro) │ HN (filtro)  │ HN       │ HN          │
│ Perfil ind. │ Perfil ind.  │Perfil ind│ Perfil ind. │
└─────────────┴──────────────┴──────────┴─────────────┘
```

### Configuración por sección

```yaml
sections:
  - name: cybersecurity
    display_name: "🔒 Cybersecurity"
    enabled: true
    max_briefing_articles: 5
    seed_keywords:
      - "CVE vulnerability exploit"
      - "ransomware malware threat"
      - "kubernetes security RBAC"
      - "zero-day attack"
      - "data breach incident"
      - "cloud security posture"

  - name: tech
    display_name: "💻 Tech"
    enabled: true
    max_briefing_articles: 5
    seed_keywords:
      - "kubernetes container orchestration"
      - "golang Go programming"
      - "LLM AI model release"
      - "self-hosted open source"
      - "linux kernel development"
      - "cloud native infrastructure"

  - name: economy
    display_name: "📈 Economy"
    enabled: true
    max_briefing_articles: 3
    seed_keywords:
      - "NVIDIA stock earnings semiconductor"
      - "Bitcoin cryptocurrency market"
      - "tech stock earnings revenue"
      - "Federal Reserve interest rates"
      - "IPO valuation funding"
      - "S&P 500 market analysis"

  - name: world
    display_name: "🌍 World"
    enabled: true
    max_briefing_articles: 2
    seed_keywords:
      - "geopolitical conflict major event"
      - "climate disaster emergency"
      - "election government change"
      - "pandemic health crisis"
      - "international treaty sanctions"
```

---

## 🔒 Cybersecurity

### RSS Feeds

| Fuente | URL del Feed | Descripción | Señal/Ruido |
|---|---|---|---|
| tl;dr sec (Clint Gibler) | `tldrsec.com/feed` | Newsletter semanal curada por un security researcher. La mejor relación señal/ruido en infosec. | ⭐⭐⭐⭐⭐ |
| Krebs on Security | `krebsonsecurity.com/feed/` | Brian Krebs, periodista investigativo de ciberseguridad. Rompe noticias de breaches y cibercrimen. | ⭐⭐⭐⭐⭐ |
| The Hacker News (THN) | `feeds.feedburner.com/TheHackersNews` | Noticias diarias de seguridad. Alto volumen pero buena cobertura. No confundir con Hacker News (YC). | ⭐⭐⭐⭐ |
| BleepingComputer | `bleepingcomputer.com/feed/` | Noticias de seguridad, malware, vulnerabilidades. Cobertura muy rápida de incidentes. | ⭐⭐⭐⭐ |
| Schneier on Security | `schneier.com/feed/` | Bruce Schneier, criptógrafo y pensador de seguridad. Más análisis y opinión que noticias puras. | ⭐⭐⭐⭐⭐ |
| SANS Internet Storm Center | `isc.sans.edu/rssfeed.xml` | Diario de amenazas activas en tiempo real. Técnico y operacional. Lo usan SOCs profesionales. | ⭐⭐⭐⭐ |
| Troy Hunt | `troyhunt.com/rss/` | Creador de Have I Been Pwned. Análisis de brechas de datos, seguridad web, y opinión de la industria. | ⭐⭐⭐⭐⭐ |
| Daniel Miessler (Unsupervised Learning) | `danielmiessler.com/feed/` | Newsletter semanal que mezcla ciberseguridad, IA y reflexiones tech. Análisis curado, no noticias puras. | ⭐⭐⭐⭐⭐ |
| Risky Business | `risky.biz/feeds/risky-business/` | El podcast de referencia en infosec. Patrick Gray entrevista a figuras clave. Tiene newsletter asociada. | ⭐⭐⭐⭐⭐ |
| TLDR InfoSec | `tldr.tech/infosec/rss` | Edición de seguridad de TLDR. 3-5 noticias diarias curadas. Excelente filtro. | ⭐⭐⭐⭐⭐ |
| Dark Reading | `darkreading.com/rss.xml` | Medio de referencia en ciberseguridad empresarial. Más volumen, pero cubre todo. | ⭐⭐⭐ |
| OpenShift Security Advisories | `access.redhat.com/errata-search/rss` | Advisories de seguridad de Red Hat (incluye OpenShift). Directamente relevante para Kyndryl. | ⭐⭐⭐⭐⭐ |

### Subreddits

| Subreddit | Descripción | Señal/Ruido |
|---|---|---|
| r/netsec | Seguridad ofensiva/defensiva técnica. La mejor comunidad de infosec en Reddit. | ⭐⭐⭐⭐⭐ |
| r/cybersecurity | Más generalista que netsec. Noticias, carrera, discusiones. | ⭐⭐⭐⭐ |
| r/AskNetsec | Preguntas y respuestas de seguridad. Útil para detectar tendencias y preocupaciones. | ⭐⭐⭐ |
| r/blueteamsec | Seguridad defensiva específica. Detección, respuesta a incidentes, SIEM. | ⭐⭐⭐⭐ |

### Hacker News

HN no se filtra por sección a nivel de ingesta — se ingesta todo y el motor de relevancia asigna artículos de seguridad a esta sección basándose en embeddings + keywords.

---

## 💻 Tech

### RSS Feeds

| Fuente | URL del Feed | Descripción | Señal/Ruido |
|---|---|---|---|
| TLDR Newsletter | `tldr.tech/tech/rss` | 5-7 noticias tech diarias curadas. El filtro humano más eficiente que existe. | ⭐⭐⭐⭐⭐ |
| TLDR AI | `tldr.tech/ai/rss` | Edición IA de TLDR. Lanzamientos de modelos, papers, herramientas. | ⭐⭐⭐⭐⭐ |
| Ars Technica | `feeds.arstechnica.com/arstechnica/index` | Medio tech con profundidad técnica real. Cubre seguridad, ciencia, gaming, política tech. | ⭐⭐⭐⭐ |
| The Verge | `theverge.com/rss/index.xml` | Tech generalista. Lanzamientos, adquisiciones, industria. Menos técnico, más producto. | ⭐⭐⭐ |
| Lobsters (lobste.rs) | `lobste.rs/rss` | Como HN pero solo por invitación. Más técnico, menos ruido, cero startups/drama. | ⭐⭐⭐⭐⭐ |
| LWN.net | `lwn.net/headlines/rss` | Linux y kernel. La fuente definitiva para desarrollo de sistemas. | ⭐⭐⭐⭐⭐ |
| Go Blog | `blog.golang.org/feed.atom` | Blog oficial de Go. Releases, best practices, diseño del lenguaje. | ⭐⭐⭐⭐⭐ |
| Kubernetes Blog | `kubernetes.io/feed.xml` | Blog oficial de Kubernetes. Releases, KEPs, guías. | ⭐⭐⭐⭐⭐ |
| Red Hat OpenShift Blog | `redhat.com/en/rss/blog/channel/red-hat-openshift` | Blog oficial. Releases, features, arquitectura. Directamente relevante para tu trabajo. | ⭐⭐⭐⭐⭐ |
| Red Hat Developer Blog | `developers.redhat.com/blog/feed` | OpenShift + middleware + Kubernetes desde perspectiva de desarrollo. | ⭐⭐⭐⭐ |
| stderr.at (OpenShift) | `blog.stderr.at/index.xml` | Dos arquitectos de Red Hat Austria. Guías prácticas de OpenShift, GitOps, networking. | ⭐⭐⭐⭐⭐ |
| OKD Blog | `okd.io/blog/index.xml` | La distribución community de OpenShift. Relevante para entender la upstream. | ⭐⭐⭐⭐ |
| Papers We Love | `paperswelove.org/feed.xml` | Papers académicos de CS curados por la comunidad. Menos activo pero de alta calidad. | ⭐⭐⭐ |
| Ollama Blog | via `Olshansk/rss-feeds` | Noticias y releases de Ollama (LLMs locales). | ⭐⭐⭐⭐ |

### RSS Feeds — AI Labs (vía Olshansk/rss-feeds o nativos)

| Lab | Feed | Método | Descripción |
|---|---|---|---|
| Anthropic News | `Olshansk/rss-feeds` → `feed_anthropic_news.xml` | Scraped (hourly) | Anuncios de productos, partnerships, políticas |
| Anthropic Engineering | `Olshansk/rss-feeds` → `feed_anthropic_engineering.xml` | Scraped (hourly) | Posts técnicos del equipo de ingeniería |
| Anthropic Research | `Olshansk/rss-feeds` → `feed_anthropic_research.xml` | Scraped (hourly) | Papers y resultados de investigación |
| Claude Blog | `Olshansk/rss-feeds` → `feed_claude.xml` | Scraped (hourly) | Updates específicos de Claude |
| OpenAI News | `openai.com/news/rss.xml` | Nativo ✅ | Anuncios oficiales de OpenAI |
| OpenAI Research | `Olshansk/rss-feeds` → `feed_openai_research.xml` | Scraped (hourly) | Papers y research posts |
| xAI News | `Olshansk/rss-feeds` → `feed_xainews.xml` | Scraped (hourly) | Noticias de xAI/Grok |
| Google DeepMind | `research.google/blog/rss` | Nativo ✅ | Research y anuncios de DeepMind |
| Zhipu/GLM | `github.com/zai-org/GLM-4.5` releases | GitHub API | Releases y changelogs (no tiene blog RSS) |
| Moonshot/Kimi | `github.com/moonshotai` releases | GitHub API | Releases y changelogs (blog sin RSS) |

**Nota sobre Zhipu y Moonshot:** No tienen RSS. Se monitorizan vía GitHub Releases API (Fase 4) y el "efecto difusión" — sus noticias importantes llegan a r/LocalLLaMA y HN en horas. Si se pierde algo, se puede añadir un scraper custom de `z.ai/blog` y `platform.moonshot.ai/blog` más adelante.

### Subreddits

| Subreddit | Descripción | Señal/Ruido |
|---|---|---|
| r/kubernetes | Discusiones, troubleshooting, noticias del ecosistema K8s. | ⭐⭐⭐⭐ |
| r/selfhosted | Proyectos y herramientas self-hosted. Tu comunidad target para Flux. | ⭐⭐⭐⭐ |
| r/homelab | Hardware, builds, infraestructura doméstica. | ⭐⭐⭐ |
| r/LocalLLaMA | LLMs locales, releases de modelos, benchmarks. El hub de noticias de IA open-source. | ⭐⭐⭐⭐⭐ |
| r/MachineLearning | Papers, discusiones académicas, lanzamientos. Más formal que LocalLLaMA. | ⭐⭐⭐⭐ |
| r/golang | Noticias, librerías, discusiones sobre Go. | ⭐⭐⭐⭐ |
| r/linux | Noticias de Linux, distros, kernel. Alto volumen. | ⭐⭐⭐ |
| r/openshift | Comunidad pequeña pero directamente relevante para tu trabajo. | ⭐⭐⭐⭐ |

---

## 📈 Economy

### RSS Feeds

| Fuente | URL del Feed | Descripción | Señal/Ruido |
|---|---|---|---|
| TLDR Founders | `tldr.tech/founders/rss` | Startups, funding, mercado tech. Curado. | ⭐⭐⭐⭐ |
| Bloomberg Technology | `feeds.bloomberg.com/technology/news.rss` | Noticias financieras de empresas tech. NVIDIA, Apple, Google earnings. | ⭐⭐⭐⭐ |
| Reuters Business | `feeds.reuters.com/reuters/businessNews` | Noticias de negocios globales. Fiable, neutral. | ⭐⭐⭐⭐ |
| CNBC Tech | `cnbc.com/id/19854910/device/rss/rss.html` | Mercados + tech. Cobertura de earnings, IPOs, adquisiciones. | ⭐⭐⭐ |
| CoinDesk | `coindesk.com/arc/outboundfeeds/rss/` | Crypto y blockchain. La fuente más establecida. | ⭐⭐⭐⭐ |
| The Block | `theblock.co/rss.xml` | Crypto/DeFi con enfoque más analítico que CoinDesk. | ⭐⭐⭐⭐ |
| Finimize | `finimize.com/wp/feed/` | Explicaciones simples de noticias financieras complejas. | ⭐⭐⭐⭐ |
| Financial Times Tech | `ft.com/technology?format=rss` | FT sección tech. Paywall parcial pero el RSS da título + resumen. | ⭐⭐⭐⭐ |
| Expansion (España) | `expansion.com/rss/portada.html` | Economía española y europea. Relevante por tu ubicación. | ⭐⭐⭐ |

### Subreddits

| Subreddit | Descripción | Señal/Ruido |
|---|---|---|
| r/stocks | Análisis de acciones, earnings, mercado general. | ⭐⭐⭐⭐ |
| r/wallstreetbets | Alto ruido pero detecta movimientos y sentiment retail rápidamente. Filtrar agresivamente. | ⭐⭐ |
| r/CryptoCurrency | Noticias crypto, análisis, discusiones. Volumen alto. | ⭐⭐⭐ |
| r/investing | Inversiones long-term, estrategia, análisis fundamental. Más serio que stocks. | ⭐⭐⭐⭐ |
| r/economics | Macroeconomía, política monetaria, análisis. Más académico. | ⭐⭐⭐⭐ |
| r/nvidia | Noticias, earnings, productos NVIDIA. Directamente relevante por tu interés en IA + inversiones. | ⭐⭐⭐ |

---

## 🌍 World

### RSS Feeds

| Fuente | URL del Feed | Descripción | Señal/Ruido |
|---|---|---|---|
| Reuters Top News | `feeds.reuters.com/reuters/topNews` | Las noticias globales más importantes. Neutral, fiable, conciso. | ⭐⭐⭐⭐⭐ |
| BBC World News | `feeds.bbci.co.uk/news/world/rss.xml` | Cobertura global amplia. El estándar de noticias internacionales. | ⭐⭐⭐⭐⭐ |
| AP News | `apnews.com/index.rss` | Associated Press. Wire service puro. Solo hechos, mínima opinión. | ⭐⭐⭐⭐⭐ |
| El País Internacional | `feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/internacional/portada` | Noticias internacionales en español. Relevante por tu ubicación. | ⭐⭐⭐⭐ |
| The Guardian World | `theguardian.com/world/rss` | Cobertura global con buen análisis. Ligeramente editorial. | ⭐⭐⭐⭐ |
| Al Jazeera | `aljazeera.com/xml/rss/all.xml` | Perspectiva no-occidental. Útil para contraste con fuentes anglo. | ⭐⭐⭐⭐ |

### Subreddits

| Subreddit | Descripción | Señal/Ruido |
|---|---|---|
| r/worldnews | Noticias globales. Muy alto volumen pero los top posts suelen ser significativos. | ⭐⭐⭐ |
| r/geopolitics | Análisis geopolítico serio. Bajo volumen, alta calidad. | ⭐⭐⭐⭐⭐ |
| r/europe | Noticias europeas. Relevante por tu ubicación y planes de mudanza. | ⭐⭐⭐⭐ |

---

## Fuentes Compartidas (multi-sección)

Algunas fuentes generan contenido que pertenece a múltiples secciones. El motor de relevancia asigna cada artículo a la sección más apropiada basándose en embeddings:

| Fuente | Secciones posibles | Notas |
|---|---|---|
| Hacker News (API) | Tech, Cybersecurity, Economy | Un post sobre un CVE → Cyber. Un launch de startup → Tech/Economy |
| Reddit r/technology | Tech, Economy | Noticias tech con impacto económico |
| TLDR Newsletter | Tech, Cybersecurity | La edición principal mezcla ambas |
| Ars Technica | Tech, Cybersecurity, World | Cubre política tech que cruza con World |
| Reuters | Economy, World | Dependiendo del artículo |

### Lógica de asignación

1. Si la fuente pertenece a UNA sola sección → asignación directa
2. Si la fuente pertenece a VARIAS secciones → el motor de relevancia calcula similaridad coseno contra los seed keywords de cada sección y asigna a la más alta
3. Si un artículo es relevante para >1 sección con scores similares → aparece en ambas (con dedup visual en el briefing)

---

## Hacker News — Tratamiento Especial

HN no se asigna a una sección fija. Se ingesta globalmente y cada artículo se clasifica en la sección más relevante:

- Posts sobre CVEs, breaches, malware → 🔒 Cybersecurity
- Posts sobre LLMs, Go, K8s, self-hosted → 💻 Tech
- Posts sobre earnings, crypto, mercados → 📈 Economy
- Posts sobre eventos globales importantes → 🌍 World

El filtro de HN por score (>10 por defecto) ya reduce mucho el ruido. La clasificación por sección la hace el motor de embeddings, no GLM (para ahorrar tokens).

---

## Resumen de Volumen Estimado

| Sección | Feeds RSS | Subreddits | Artículos/día estimados (pre-filtro) | Artículos en briefing |
|---|---|---|---|---|
| 🔒 Cybersecurity | 12 | 4 | ~80-120 | 5 |
| 💻 Tech | 14 + 10 AI labs | 8 | ~150-250 | 5 |
| 📈 Economy | 9 | 6 | ~100-180 | 3 |
| 🌍 World | 6 | 3 | ~80-150 | 2 |
| **Total** | **~51** | **~21** | **~400-700** | **~15** |

El filtrado por embeddings descarta ~80% → ~80-140 artículos pasan a GLM → GLM selecciona los ~15 mejores → briefing matutino.

---

## Impacto en la Arquitectura

### Cambios en el schema

```sql
-- Nueva tabla de secciones
CREATE TABLE sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,        -- 'cybersecurity', 'tech', 'economy', 'world'
    display_name TEXT NOT NULL,       -- '🔒 Cybersecurity'
    enabled BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    max_briefing_articles INTEGER DEFAULT 5,
    seed_keywords TEXT[],             -- Para cold start del perfil
    config JSONB                      -- Configuración extra
);

-- Relación fuentes ↔ secciones (muchos a muchos)
CREATE TABLE source_sections (
    source_id UUID REFERENCES sources(id),
    section_id UUID REFERENCES sections(id),
    PRIMARY KEY (source_id, section_id)
);

-- Artículos ahora tienen sección asignada
ALTER TABLE articles ADD COLUMN section_id UUID REFERENCES sections(id);

-- Perfil de feedback POR SECCIÓN (no global)
CREATE TABLE section_profiles (
    section_id UUID REFERENCES sections(id),
    positive_embedding vector(384),
    negative_embedding vector(384),
    like_count INTEGER DEFAULT 0,
    dislike_count INTEGER DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (section_id)
);
```

### Cambios en el briefing

El prompt de generación del briefing ahora incluye la estructura de secciones:

```
Genera un briefing matutino organizado en las siguientes secciones:

## 🔒 Cybersecurity (máx 5 artículos)
[artículos de seguridad]

## 💻 Tech (máx 5 artículos)
[artículos de tech]

## 📈 Economy (máx 3 artículos)
[artículos de economía]

## 🌍 World (máx 2 artículos)
[artículos de mundo]

Para cada sección: destaca el artículo más importante primero.
Si hay artículos relacionados entre secciones, conéctalos.
```

### Cambios en la UI

- Tabs o acordeones por sección en el briefing
- Filtro de feed por sección
- Feedback independiente por sección
- Admin: crear/editar/desactivar secciones
- Admin: asignar fuentes a secciones
