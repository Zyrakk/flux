-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Sections of the briefing (modular, can be enabled/disabled)
CREATE TABLE sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    max_briefing_articles INTEGER DEFAULT 5,
    seed_keywords TEXT[],
    config JSONB
);

-- Configured content sources
CREATE TABLE sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type TEXT NOT NULL,
    name TEXT NOT NULL,
    config JSONB NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    last_fetched_at TIMESTAMPTZ,
    error_count INTEGER DEFAULT 0,
    last_error TEXT
);

-- Many-to-many: sources <-> sections
CREATE TABLE source_sections (
    source_id UUID REFERENCES sources(id) ON DELETE CASCADE,
    section_id UUID REFERENCES sections(id) ON DELETE CASCADE,
    PRIMARY KEY (source_id, section_id)
);

-- Ingested articles
CREATE TABLE articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    section_id UUID REFERENCES sections(id),
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT,
    summary TEXT,
    author TEXT,
    published_at TIMESTAMPTZ,
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    embedding vector(384),
    relevance_score FLOAT,
    categories TEXT[],
    status TEXT DEFAULT 'pending',
    metadata JSONB,
    UNIQUE(source_type, source_id)
);

CREATE INDEX idx_articles_status ON articles(status);
CREATE INDEX idx_articles_published ON articles(published_at DESC);
CREATE INDEX idx_articles_section ON articles(section_id);
CREATE INDEX idx_articles_ingested ON articles(ingested_at DESC);
CREATE INDEX idx_articles_embedding ON articles USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Generated briefings
CREATE TABLE briefings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generated_at TIMESTAMPTZ DEFAULT NOW(),
    content TEXT NOT NULL,
    article_ids UUID[],
    metadata JSONB
);

CREATE INDEX idx_briefings_generated ON briefings(generated_at DESC);

-- User feedback on articles
CREATE TABLE feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID REFERENCES articles(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_feedback_article ON feedback(article_id);
CREATE INDEX idx_feedback_action ON feedback(action);

-- Per-section relevance profile (built from user feedback)
CREATE TABLE section_profiles (
    section_id UUID REFERENCES sections(id) ON DELETE CASCADE,
    positive_embedding vector(384),
    negative_embedding vector(384),
    like_count INTEGER DEFAULT 0,
    dislike_count INTEGER DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (section_id)
);

-- Seed data: initial sections
INSERT INTO sections (name, display_name, sort_order, max_briefing_articles, seed_keywords) VALUES
    ('cybersecurity', '🔒 Cybersecurity', 1, 5, ARRAY[
        'CVE vulnerability exploit',
        'ransomware malware threat',
        'kubernetes security RBAC',
        'zero-day attack',
        'data breach incident',
        'cloud security posture'
    ]),
    ('tech', '💻 Tech', 2, 5, ARRAY[
        'kubernetes container orchestration',
        'golang Go programming',
        'LLM AI model release',
        'self-hosted open source',
        'linux kernel development',
        'cloud native infrastructure'
    ]),
    ('economy', '📈 Economy', 3, 3, ARRAY[
        'NVIDIA stock earnings semiconductor',
        'Bitcoin cryptocurrency market',
        'tech stock earnings revenue',
        'Federal Reserve interest rates',
        'IPO valuation funding',
        'S&P 500 market analysis'
    ]),
    ('world', '🌍 World', 4, 2, ARRAY[
        'geopolitical conflict major event',
        'climate disaster emergency',
        'election government change',
        'pandemic health crisis',
        'international treaty sanctions'
    ]);
