-- Seed RSS and Hacker News sources for Phase 1
WITH source_rows (name, source_type, config, section_names) AS (
    VALUES
        -- Cybersecurity
        ('tl;dr sec', 'rss', '{"url":"https://tldrsec.com/feed","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Krebs on Security', 'rss', '{"url":"https://krebsonsecurity.com/feed/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('The Hacker News', 'rss', '{"url":"https://feeds.feedburner.com/TheHackersNews","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('BleepingComputer', 'rss', '{"url":"https://www.bleepingcomputer.com/feed/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Schneier on Security', 'rss', '{"url":"https://www.schneier.com/feed/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('SANS Internet Storm Center', 'rss', '{"url":"https://isc.sans.edu/rssfeed.xml","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Troy Hunt', 'rss', '{"url":"https://www.troyhunt.com/rss/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Daniel Miessler', 'rss', '{"url":"https://danielmiessler.com/feed/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Risky Business', 'rss', '{"url":"https://risky.biz/feeds/risky-business/","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('TLDR InfoSec', 'rss', '{"url":"https://tldr.tech/infosec/rss","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Dark Reading', 'rss', '{"url":"https://www.darkreading.com/rss.xml","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('OpenShift Security Advisories', 'rss', '{"url":"https://access.redhat.com/errata-search/rss","format":"rss"}'::jsonb, ARRAY['cybersecurity']::text[]),

        -- Tech core feeds
        ('TLDR Newsletter', 'rss', '{"url":"https://tldr.tech/tech/rss","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('TLDR AI', 'rss', '{"url":"https://tldr.tech/ai/rss","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Ars Technica', 'rss', '{"url":"https://feeds.arstechnica.com/arstechnica/index","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('The Verge', 'rss', '{"url":"https://www.theverge.com/rss/index.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Lobsters', 'rss', '{"url":"https://lobste.rs/rss","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('LWN.net', 'rss', '{"url":"https://lwn.net/headlines/rss","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Go Blog', 'rss', '{"url":"https://blog.golang.org/feed.atom","format":"atom"}'::jsonb, ARRAY['tech']::text[]),
        ('Kubernetes Blog', 'rss', '{"url":"https://kubernetes.io/feed.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Red Hat OpenShift Blog', 'rss', '{"url":"https://www.redhat.com/en/rss/blog/channel/red-hat-openshift","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Red Hat Developer Blog', 'rss', '{"url":"https://developers.redhat.com/blog/feed","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('stderr.at', 'rss', '{"url":"https://blog.stderr.at/index.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('OKD Blog', 'rss', '{"url":"https://okd.io/blog/index.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Papers We Love', 'rss', '{"url":"https://paperswelove.org/feed.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Ollama Blog', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_ollama.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),

        -- Tech AI labs
        ('Anthropic News', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_news.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Anthropic Engineering', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_engineering.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Anthropic Research', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_research.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Anthropic Frontier Red Team', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_red.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Claude Blog', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_claude.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Claude Code Changelog', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_changelog_claude_code.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('OpenAI News', 'rss', '{"url":"https://openai.com/news/rss.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('OpenAI Research', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_openai_research.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('xAI News', 'rss', '{"url":"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_xainews.xml","format":"rss"}'::jsonb, ARRAY['tech']::text[]),
        ('Google DeepMind', 'rss', '{"url":"https://research.google/blog/rss","format":"rss"}'::jsonb, ARRAY['tech']::text[]),

        -- Economy
        ('TLDR Founders', 'rss', '{"url":"https://tldr.tech/founders/rss","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('Bloomberg Technology', 'rss', '{"url":"https://feeds.bloomberg.com/technology/news.rss","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('Reuters Business', 'rss', '{"url":"https://feeds.reuters.com/reuters/businessNews","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('CNBC Tech', 'rss', '{"url":"https://www.cnbc.com/id/19854910/device/rss/rss.html","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('CoinDesk', 'rss', '{"url":"https://www.coindesk.com/arc/outboundfeeds/rss/","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('The Block', 'rss', '{"url":"https://www.theblock.co/rss.xml","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('Finimize', 'rss', '{"url":"https://www.finimize.com/wp/feed/","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('Financial Times Tech', 'rss', '{"url":"https://www.ft.com/technology?format=rss","format":"rss"}'::jsonb, ARRAY['economy']::text[]),
        ('Expansion', 'rss', '{"url":"https://www.expansion.com/rss/portada.html","format":"rss"}'::jsonb, ARRAY['economy']::text[]),

        -- World
        ('Reuters Top News', 'rss', '{"url":"https://feeds.reuters.com/reuters/topNews","format":"rss"}'::jsonb, ARRAY['world']::text[]),
        ('BBC World News', 'rss', '{"url":"https://feeds.bbci.co.uk/news/world/rss.xml","format":"rss"}'::jsonb, ARRAY['world']::text[]),
        ('AP News', 'rss', '{"url":"https://apnews.com/index.rss","format":"rss"}'::jsonb, ARRAY['world']::text[]),
        ('El Pais Internacional', 'rss', '{"url":"https://feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/internacional/portada","format":"rss"}'::jsonb, ARRAY['world']::text[]),
        ('The Guardian World', 'rss', '{"url":"https://www.theguardian.com/world/rss","format":"rss"}'::jsonb, ARRAY['world']::text[]),
        ('Al Jazeera', 'rss', '{"url":"https://www.aljazeera.com/xml/rss/all.xml","format":"rss"}'::jsonb, ARRAY['world']::text[]),

        -- Hacker News (multi-section source)
        ('Hacker News', 'hn', '{"api_base":"https://hacker-news.firebaseio.com/v0"}'::jsonb, ARRAY['cybersecurity', 'tech', 'economy', 'world']::text[])
),
inserted_sources AS (
    INSERT INTO sources (source_type, name, config, enabled)
    SELECT source_type, name, config, TRUE
    FROM source_rows
    RETURNING id, name
)
INSERT INTO source_sections (source_id, section_id)
SELECT ins.id, sec.id
FROM inserted_sources ins
JOIN source_rows src ON src.name = ins.name
JOIN LATERAL unnest(src.section_names) AS sec_name(name) ON TRUE
JOIN sections sec ON sec.name = sec_name.name;
