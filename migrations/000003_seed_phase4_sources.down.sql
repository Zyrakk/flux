DELETE FROM sources
WHERE (source_type = 'reddit' AND name IN (
    'Reddit r/netsec',
    'Reddit r/cybersecurity',
    'Reddit r/AskNetsec',
    'Reddit r/blueteamsec',
    'Reddit r/kubernetes',
    'Reddit r/selfhosted',
    'Reddit r/homelab',
    'Reddit r/LocalLLaMA',
    'Reddit r/MachineLearning',
    'Reddit r/golang',
    'Reddit r/linux',
    'Reddit r/openshift',
    'Reddit r/stocks',
    'Reddit r/wallstreetbets',
    'Reddit r/CryptoCurrency',
    'Reddit r/investing',
    'Reddit r/economics',
    'Reddit r/nvidia',
    'Reddit r/worldnews',
    'Reddit r/geopolitics',
    'Reddit r/europe'
))
OR (source_type = 'github' AND name IN (
    'GitHub kubernetes/kubernetes',
    'GitHub traefik/traefik',
    'GitHub nats-io/nats-server',
    'GitHub argoproj/argo-cd',
    'GitHub grafana/grafana',
    'GitHub prometheus/prometheus',
    'GitHub ollama/ollama',
    'GitHub THUDM/GLM-4'
));
