-- Seed Reddit and GitHub sources for Phase 4
WITH source_rows (name, source_type, config, section_names) AS (
    VALUES
        -- Reddit: Cybersecurity
        ('Reddit r/netsec', 'reddit', '{"subreddit":"netsec","min_score":20,"sort":"hot"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Reddit r/cybersecurity', 'reddit', '{"subreddit":"cybersecurity","min_score":20,"sort":"hot"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Reddit r/AskNetsec', 'reddit', '{"subreddit":"AskNetsec","min_score":8,"sort":"hot"}'::jsonb, ARRAY['cybersecurity']::text[]),
        ('Reddit r/blueteamsec', 'reddit', '{"subreddit":"blueteamsec","min_score":10,"sort":"hot"}'::jsonb, ARRAY['cybersecurity']::text[]),

        -- Reddit: Tech
        ('Reddit r/kubernetes', 'reddit', '{"subreddit":"kubernetes","min_score":15,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/selfhosted', 'reddit', '{"subreddit":"selfhosted","min_score":20,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/homelab', 'reddit', '{"subreddit":"homelab","min_score":25,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/LocalLLaMA', 'reddit', '{"subreddit":"LocalLLaMA","min_score":40,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/MachineLearning', 'reddit', '{"subreddit":"MachineLearning","min_score":30,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/golang', 'reddit', '{"subreddit":"golang","min_score":15,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/linux', 'reddit', '{"subreddit":"linux","min_score":30,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),
        ('Reddit r/openshift', 'reddit', '{"subreddit":"openshift","min_score":8,"sort":"hot"}'::jsonb, ARRAY['tech']::text[]),

        -- Reddit: Economy
        ('Reddit r/stocks', 'reddit', '{"subreddit":"stocks","min_score":40,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),
        ('Reddit r/wallstreetbets', 'reddit', '{"subreddit":"wallstreetbets","min_score":100,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),
        ('Reddit r/CryptoCurrency', 'reddit', '{"subreddit":"CryptoCurrency","min_score":80,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),
        ('Reddit r/investing', 'reddit', '{"subreddit":"investing","min_score":30,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),
        ('Reddit r/economics', 'reddit', '{"subreddit":"economics","min_score":35,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),
        ('Reddit r/nvidia', 'reddit', '{"subreddit":"nvidia","min_score":20,"sort":"hot"}'::jsonb, ARRAY['economy']::text[]),

        -- Reddit: World
        ('Reddit r/worldnews', 'reddit', '{"subreddit":"worldnews","min_score":250,"sort":"hot"}'::jsonb, ARRAY['world']::text[]),
        ('Reddit r/geopolitics', 'reddit', '{"subreddit":"geopolitics","min_score":25,"sort":"hot"}'::jsonb, ARRAY['world']::text[]),
        ('Reddit r/europe', 'reddit', '{"subreddit":"europe","min_score":40,"sort":"hot"}'::jsonb, ARRAY['world']::text[]),

        -- GitHub Releases (mostly Tech)
        ('GitHub kubernetes/kubernetes', 'github', '{"repo":"kubernetes/kubernetes"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub traefik/traefik', 'github', '{"repo":"traefik/traefik"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub nats-io/nats-server', 'github', '{"repo":"nats-io/nats-server"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub argoproj/argo-cd', 'github', '{"repo":"argoproj/argo-cd"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub grafana/grafana', 'github', '{"repo":"grafana/grafana"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub prometheus/prometheus', 'github', '{"repo":"prometheus/prometheus"}'::jsonb, ARRAY['tech']::text[]),
        ('GitHub ollama/ollama', 'github', '{"repo":"ollama/ollama"}'::jsonb, ARRAY['tech', 'economy']::text[]),
        ('GitHub THUDM/GLM-4', 'github', '{"repo":"THUDM/GLM-4"}'::jsonb, ARRAY['tech', 'economy']::text[])
),
inserted_sources AS (
    INSERT INTO sources (source_type, name, config, enabled)
    SELECT src.source_type, src.name, src.config, TRUE
    FROM source_rows src
    WHERE NOT EXISTS (
        SELECT 1
        FROM sources s
        WHERE s.source_type = src.source_type
          AND s.name = src.name
    )
    RETURNING id, name, source_type
)
INSERT INTO source_sections (source_id, section_id)
SELECT ins.id, sec.id
FROM inserted_sources ins
JOIN source_rows src ON src.name = ins.name AND src.source_type = ins.source_type
JOIN LATERAL unnest(src.section_names) AS sec_name(name) ON TRUE
JOIN sections sec ON sec.name = sec_name.name;
