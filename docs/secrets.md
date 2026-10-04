# Gestão de segredos por cliente (age)

O repositório só armazena ciphertext. Valores em claro existem apenas na
memória do worker em runtime, ou em arquivos fora do git. Criptografia
assimétrica com [age](https://github.com/filosottile/age): quem criptografa
precisa só da chave **pública**; só o ambiente de execução detém a privada.

## Estrutura

| Caminho | Conteúdo | No git? |
|---|---|---|
| `env/<client>.env.example` | Nomes das variáveis (template) | Sim |
| `env/<client>.env` | Valores reais, dev local | Não (`env/*.env` no `.gitignore`) |
| `secrets/<client>/env.age` | Mesmo `.env`, criptografado | Sim (ciphertext; pode exigir `git add -f`) |
| `~/.secrets/` (operador/servidor) | `.env` de produção e/ou `*.age.key` (chmod 600) | Nunca |
| `*.age.key` | Identidade privada age | Nunca, em lugar nenhum versionado |

Workflow JSON referencia só **nomes** (`"auth_secret": "ACME_X"`). O teste
`TestRecipeStructure` quebra o build se um valor com cara de segredo
(`hooks.slack…`, `xoxb-`, `sk-`, `ghp_`, PEM) aparecer em qualquer receita.

## Chaves: uma por ambiente, por cliente quando preciso

- **Padrão:** uma chave da consultoria por ambiente de execução
  (laptop, servidor). Um arquivo por cliente, todos para o mesmo
  recipiente — simples de operar.
- **Por cliente:** gere um par dedicado quando o cliente exigir
  isolamento ou compliance (stage 3 em `docs/tenancy-evolution.md`).
  O loader não distingue: cada host usa seu `AGE_IDENTITY_FILE`.

```bash
age-keygen -o ~/.secrets/consultancy.age.key   # uma vez por máquina
chmod 600 ~/.secrets/consultancy.age.key
age-keygen -y ~/.secrets/consultancy.age.key   # imprime a pública
```

## Ciclo: criar → editar → versionar

Edição sempre via `scripts/edit-secrets.sh <client>` — descriptografa
para um temp dir com trap de destruição (inclui swap do editor),
re-criptografa para um recipiente derivado da sua própria identidade
(nunca cifra para uma chave que você não abre), avisa sobre chaves fora
do template e verifica a descriptografia antes de sair:

```bash
make edit-secrets CLIENT=acme
# …edita no $EDITOR, salva, sai…
git add -f secrets/acme/env.age && git commit
```

Bootstrap (primeira vez, sem ciphertext ainda): o script parte do
`env/<client>.env.example`. O histórico git é o backup — o ciphertext
anterior continua commitado até o próximo commit.

## Cheat-sheet age (quando precisar do manual)

```bash
age-keygen -o ~/.secrets/x.age.key && chmod 600 ~/.secrets/x.age.key
age-keygen -y ~/.secrets/x.age.key                    # pública
age -r age1PUBLICA -o secrets/acme/env.age env/acme.env
age --decrypt -i ~/.secrets/x.age.key -o - secrets/acme/env.age | head -c 200  # confere
```

Prefira o script ao manual: o ciclo manual esquece `shred` do temp e
deixa backup/swap do editor para trás.

## Runtime: descriptografia just-in-time, sem `.env` em disco

O `internal/secret` descriptografa via binário `age` e faz merge **em
memória** — o worker nunca precisa de um `.env` descriptografado no disco.
Evite o padrão `age --decrypt … > .env`; se um entrypoint precisar de
arquivo, use tmpfs e destrua após o boot (e o `.gitignore` já cobre
`.env` e `env/*.env`).

```bash
# Dev local: arquivo do cliente direto
CLIENT_ID=acme go run ./cmd/worker

# Servidor / container: ciphertext + identidade fora do repo
export SECRETS_AGE_FILE=/run/secrets/acme/env.age
export AGE_IDENTITY_FILE=/run/secrets/consultancy.age.key
./worker   # merge em memória; env já exportado tem precedência
```

```dockerfile
# Exemplo de entrypoint que só monta segredos (nunca bake na imagem)
# docker run -v ~/.secrets:/run/secrets:ro -e SECRETS_AGE_FILE=/run/secrets/acme/env.age \
#   -e AGE_IDENTITY_FILE=/run/secrets/consultancy.age.key worker
```

## Rotação

Vazou ou trocou de máquina: gere um novo par, re-criptografe cada
`secrets/<client>/env.age` para a nova pública, commite, destrua a
privada antiga nos hosts. Rotação é por **arquivo** — nenhum código muda.

## sops: avaliação

[`sops`](https://github.com/getsops/sops) também usa age por baixo, mas
adiciona edição in-place (`sops arquivo` abre o editor, descriptografa
em temp, re-criptografa ao salvar), múltiplos recipientes e rotação de
chaves (`sops updatekeys` / `rotate`).

**Veredito (revisto com o ambiente real): manter age nativo.** Três
motivos, o primeiro decisivo:

1. **sops não edita arquivos age puros.** Adotá-lo trocaria o formato
   (envelope sops) — e aí o runtime muda junto: `age --decrypt` do
   `internal/secret` não lê envelope sops, então seria preciso binário
   sops no container (`sops exec-env`) ou mudança no loader. Custo de
   migração real, benefício zero em runtime (os valores caem nas mesmas
   env vars do mesmo jeito).
2. Os superpoderes do sops (times, múltiplos recipientes, Shamir/KMS,
   rotação frequente) não casam com operador solo editando raramente.
3. A dor real — editar sem vazar plaintext em temp/swap — já está
   resolvida sem dependência nova: `scripts/edit-secrets.sh` (ciclo
   testado: bootstrap, roundtrip, no-op, guarda de typos).

**Quando migrar para sops — gatilhos concretos, não antes:**

- segundo operador com acesso a segredos, ou
- recipientes por cliente virando rotina (stage 3 com compliance), ou
- edição semanal+ onde o ciclo manual vira atrito.

**Se um gatilho disparar, a integração é:**

1. `.sops.yaml` na raiz mapeando cada pasta ao recipiente:
   ```yaml
   creation_rules:
     - path_regex: secrets/[^/]+/\.env$
       age: age1PUBLICADACONSULTORIA
   ```
2. Criar/editar: `sops secrets/acme/prod.env` (abre `$EDITOR`; nunca
   deixe o temp vazar — sops limpa ao salvar).
3. Runtime **sem mudar código**: entrypoint com o binário sops
   disponível, sem arquivo em disco:
   ```bash
   sops exec-env secrets/acme/prod.env './worker'
   ```
   (`exec-env` injeta os valores descriptografados no ambiente e executa.)
4. **Caveat de formato (importante):** arquivo sops usa envelope próprio
   — `age --decrypt` do `internal/secret` **não** o lê. Ou o runtime usa
   `sops exec-env`/`sops --decrypt`, ou o arquivo continua em age puro.
   Não misture os dois formatos na mesma pasta.

## Higiene contra leitura por LLMs/agentes

- Agentes trabalham com **nomes** de segredos (`env/*.env.example`),
  nunca pedem nem exibem valores.
- Nunca cole conteúdo de `.env`/`~/.secrets` em chat, issue ou prompt.
- Identidades (`*.age.key`) ficam em `~/.secrets` com `600`, fora de
  qualquer repo — mesmo que um agente liste o diretório, não há o que ler.
- O scan de valores em `go test ./workflows/` é a rede de segurança:
  segredo que cair numa receita quebra o CI antes do commit.
