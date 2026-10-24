# Client intake: entrevista curta + escolha de ferramenta via degustia

Use quando o fieldbook está fino (cliente novo) ou a recipe trava numa
escolha de ferramenta (qual form? qual notifier? middleware ou consultoria?).
Degustia diagnostica e prescreve; este repo executa. Detalhe da fronteira:
`references/methodology.md`.

## 1. Fieldbook fino? Entreviste (máx 2 perguntas)

Estilo degustia Passo 3: só fatos internos do negócio, nunca "qual
ferramenta você prefere" (isso a skill decide pela hierarquia) nem nada
verificável na web (isso é websearch).

Por que 2 bastam: elas não fazem discovery (isso é FDEOps `discover`,
processo longo) — elas **desbloqueiam o build**. As 2 que quase sempre
travam uma recipe:

1. De onde vem o gatilho e com que formato? (webhook de qual sistema,
   exemplo real de payload — sem isso o `transform.json` inicial é chute)
2. Quem tem as credenciais e quem aprova o efeito colateral? (dono do
   secret, aprovador humano — sem isso nada com side-effect registra)

1 pergunta por rodada; sem resposta em 2 rodadas, assuma com suposição
explícita e registre no fieldbook. Exemplo que resolve: "o payload do
webhook HubSpot traz `dealstage`? manda um exemplo" → recipe sai na hora;
sem isso, 3 idas-e-voltas de adivinhação.

## 2. Escolha de ferramenta aberta? Consulte o catálogo degustia local

O ranker é determinístico, sem dependências, sem rede — roda contra o
checkout local do degustia (outro repo; não copiado para cá). Sem checkout,
o script degrada com a hierarquia manual em vez de falhar mudo:

```bash
./scripts/tool-pick.sh --dor "avisar o time no slack quando um deal fechar no crm" \
  --tag Automação --perfil Intermediario --k 4
# precisa de: export DEGUSTIA_SKILL_DIR=~/Development/degustia/.agents/skills/cali-degustia-diagnostico
# (ajuste o caminho para a sua máquina; sem ele, o script imprime o fallback)
```

Ordem de preferência (degustia 03-priorizacao): o que o cliente já tem >
grátis do catálogo > pago com lacuna documentada. Se a resposta for
"middleware" e o cliente vai contratar implementação (Projeto), o caminho
é consultoria via este repo (nível 0) em vez de DIY — ver
`docs/script-execution.md` Q1. Registre a decisão no fieldbook §6
(ferramenta, por que venceu, data).

## 3. O que NÃO fazer aqui

- Não chamar o MCP público do degustia deste fluxo: ele tem allowlist de
  IP (OpenAI egress; fora dela dá 403) e é stateless por design. O ranker
  local dá o mesmo resultado sem rede nem auth.
- Não colar catálogo degustia neste repo (CI própria de lá: 24 regras +
  goldens). Aponte, não copie.
