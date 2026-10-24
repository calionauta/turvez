# Onde executar? dagnats vs one-off vs Dagu vs Windmill

Resposta curta: **dagnats é o default**. Todo o resto é exceção, e a
exceção tem dono. Esta página é a árvore de decisão para o dia-a-dia;
`docs/migrate-to-windmill.md` continua sendo o plano de migração.

## Os 4 níveis (do mais barato ao mais caro de operar)

| # | Quando | Ferramenta | Custo operacional |
|---|---|---|---|
| 0 | SaaS glue, cron, approval humano, retries, DLQ | **dagnats** (este repo) | 1 stack, você já opera |
| 1 | Script one-off / teste de código gerado por LLM | `scripts/sandbox-run.sh` | zero — docker local, joga fora depois |
| 2 | Script/SSH/Docker **recorrente** que continua com cara de script | **Dagu** (CLI local, sem integrar ao repo) | 1 binário, sem DB, sem Postgres |
| 3 | Recorrente + precisa UI gerada, apps internos, catálogo de integrações | **Windmill** (migração por cliente) | Postgres + server + workers |

Níveis 2 e 3 **substituem um ao outro** na parte de orquestração —
não opere os dois em produção para o mesmo cliente. Ver comparação
oficial: `https://dagu.sh/dagu-vs-windmill`.

## Árvore de decisão (pergunte nesta ordem)

**Q1. Dá para expressar com os 12 handlers?**
`http.request`, `transform.json`, `condition`, `map`, `delay`,
`notify.*`, `request.approval`, `llm.complete`, `dlq.check`
(ver `docs/handlers-reference.md`).
Sim → nível 0. Escreva a recipe, `validate` + `go test ./workflows/`.
Ex.: HubSpot deal → Slack → Notion; invoice VIP split; refund com approval.

**Q2. É código novo que roda uma vez?** (parser que a LLM gerou, migração
de CSV, teste de API do cliente)
Sim → nível 1. Leia o script, copie para dentro do repo (o sandbox só
monta o repo em `/work:ro`), rode isolado, jogue fora:

```bash
cp /tmp/migra.py ./scratch-migra.py
./scripts/sandbox-run.sh --allow-net=off ./scratch-migra.py
```

Detalhes em `docs/sandbox-policy.md`. Se o script precisar de secrets,
passe `--secret-env` explicitamente — nunca exporte no shell do dia-a-dia.

**Q3. É comando que o cliente já tem e vai repetir?** (backup via SSH,
`deploy.sh`, `python relatorio.py`, `ffmpeg`, `dbt`, `kubectl`)
Sim, e continua com cara de script → nível 2, Dagu ad-hoc:

```bash
# instala uma vez (binário único, sem DB)
curl -fsSL https://raw.githubusercontent.com/dagucloud/dagu/main/scripts/installer.sh | bash
# workflow YAML ao lado do script do cliente, versionado junto
cat > /tmp/backup-acme.yaml <<'EOF'
schedule: "0 3 * * *"
steps:
  - id: backup
    run: ssh deploy@acme.internal /opt/backup.sh
    retry_policy: {limit: 3, interval_sec: 30}
EOF
dagu start --dags /tmp /tmp/backup-acme.yaml   # teste
dagu start-all --dags ~/.dagu/dags             # agenda + UI :8080
```

Regras: um YAML por job, logs na UI do Dagu, secrets no env do host
(nunca no YAML), sem integração com este repo. Dagu é GPLv3 — usar o
binário separado é seguro; não copie código dele para cá.

**Q4. Virou produto interno do cliente?** (precisa formulário web para
a equipe, catálogo de integrações, approvals com UI, apps low-code)
Sim → nível 3, Windmill, **por cliente** (ver
`docs/migrate-to-windmill.md` + `docs/isolating-a-client.md`):
repoint webhooks, rode em paralelo até dar verde, desligue o dagnats
para aquele cliente.

## Quando NÃO usar Dagu

- Para o que dagnats já faz (Q1 sim): dois motores = 2x monitoramento,
  2x segredos, 2x DLQ. Não pague esse preço por redundância.
- Como biblioteca dentro do worker Go (licença GPLv3 + acoplamento).
  Dagu é processo separado ou nada.
- Como substituto do `sandbox-run.sh` para one-offs: subir um scheduler
  para rodar um script uma vez é overkill.

## Exemplos da consultoria

| Pedido | Nível | Por quê |
|---|---|---|
| Deal qualified → Slack + Notion, approval >10k | 0 dagnats | SaaS glue + gate humano multi-dia |
| Nightly sync HubSpot → warehouse | 0 dagnats | cron + retries + DLQ |
| Parser Python que a LLM gerou para limpar CSV do cliente | 1 sandbox-run | roda uma vez, depois vira `transform.json` ou morre |
| Backup SSH semanal em 3 servidores do cliente | 2 Dagu | comando recorrente, Dagu nasceu para isso |
| Deploy `git pull + docker compose up` do cliente | 2 Dagu | SSH + retry + log na UI |
| Equipe do cliente quer botão "gerar relatório" com formulário | 3 Windmill | UI gerada + recursos, fora do escopo do dagnats e do Dagu simples |

## Veredito registrado

Dagu **é** alternativa à parte de orquestração do Windmill
(schedule + retry + log + UI de runs), com a vantagem de não pedir
Postgres. Não é alternativa ao dagnats neste repo (durabilidade NATS +
approvals multi-dia + 12 handlers estáveis). Manter os três documentados
e só operar um por cliente: dagnats default, Dagu ad-hoc, Windmill por
migração.
