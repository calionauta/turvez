# Sandbox policy (mínimo gratuito e suficiente)

Regra-mãe: **este runtime nunca executa script arbitrário a partir de JSON**
(`docs/writing-recipes.md` checklist, `AGENTS.md`). Sandbox externo só
entra quando o trabalho **sai** do dagnats. Todo o resto resolve com
`validate` + `go test` + run em staging.

## Decisão em 3 perguntas

**P1. A recipe usa só tasks conhecidas?** (`internal/handlers/handlers.go`)
Sim → sem sandbox externo. Rode:

```bash
dagnats workflow validate workflows/<client>/<wf>.json
go test ./workflows/   # executa cada expressão jq contra amostras
./scripts/register-workflows.sh <client>  # staging primeiro
dagnats run start "<client>/<wf>" '<json>' --watch
```

Por quê basta: `transform.json` é gojq puro sem I/O
(`internal/handlers/transform_json.go`), `condition`/`skip_if` são
comparação, `http.request`/`notify.*` têm timeout + 4xx permanente vs
5xx retryable. O risco aqui é recipe errada, não escape.

**P2. Precisa de handler Go novo?**
Só se ALL do checklist aplicar (`docs/writing-recipes.md`). Então:

1. Table test em `internal/handlers/*_test.go` (obrigatório).
2. Review humano (LLM pode propor, nunca land sem humano).
3. Rode no container worker (`distroless`, `Dockerfile.worker`) — esse
   é seu sandbox de produção, custo zero.
4. Marque custo Windmill no commit ("Windmill migration cost").

Nenhum E2B/Daytona aqui. `go test` local + container já isolam.

**P3. É script arbitrário (Python/JS/shell gerado por LLM ou legado)?**
Não entra no dagnats. Duas saídas, ambas gratuitas:

- **One-off / teste:** `scripts/sandbox-run.sh` (docker local restrito).
- **Recorrente:** virou automação → decida em `docs/script-execution.md`
  (Dagu vs Windmill vs embrulhar em handler).

## O que NÃO fazer

- Não pagar E2B ($150/mês + uso) ou Daytona para rodar jq/recipe.
  Hobby E2B ($100 one-time) só faria sentido se você passar a executar
  código não-confiável de terceiros em escala — hoje não é o caso.
- Não tratar output de `llm.complete` como código. É **dado**:
  valide schema com `transform.json`/`condition` antes de usar.
  Prompt-injection via dado do SaaS é o risco real, não falta de microVM.
- Não dar ao agent sandbox do IDE (Claude/Cursor) como controle
  multi-cliente. Útil no dia-a-dia do FDE, mas não auditável por cliente.

## `scripts/sandbox-run.sh`

```bash
# isolado por padrão: sem rede, fs read-only, sem root, limites de CPU/mem/pids
# o sandbox só monta o repo — copie o script para dentro antes, se estiver em /tmp
cp /tmp/migra.py ./scratch-migra.py
./scripts/sandbox-run.sh --allow-net=off ./scratch-migra.py
ls sandbox-out/  # script escreve em ./sandbox-out/
```

Sem docker? Rode `python3 script.py` num user dedicado sem secrets no
env (`env -i`) — documente como exceção no Fieldbook do cliente.
