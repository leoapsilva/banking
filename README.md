# Banking

Wrapper de APIs bancárias: expõe uma API unificada (boletos, bolepix, PIX,
checkout/cartão, notificações, transações) e traduz para o BaaS escolhido
pelo cliente no payload (`"baas": "c6"`). Veja `VISAO_DE_PRODUTO.md` para o
escopo de produto e `C:\Users\leoap\.claude\plans\fa-a-a-leitura-do-smooth-galaxy.md`
(ou o histórico de PRs) para o plano de implementação detalhado da Fase 1.

## Status

Fase 1 implementada: autenticação C6 (mTLS + OAuth2), checkout/cartão
(criação, autorização, consulta, cancelamento), assinatura mensal
recorrente, parcelamento anual com juros assumidos pelo cliente, e
webhooks (registro + recebimento idempotente).

Fase 2 (gate de produção): **Boleto** (`internal/boleto`), **PIX**
(`internal/pix`) e **Agendamento de Pagamentos/DDA**
(`internal/paymentscheduling`) implementados — emissão/consulta/alteração/baixa
de boleto (com juros, multa e desconto), cobrança PIX imediata e com
vencimento, consulta/listagem por txid e período, webhook próprio do
produto PIX, consulta de DDA e ciclo completo de grupo de pagamentos
(decode, consultar itens, remover itens, submeter para aprovação).
DDA/agendamento foi implementado contra a spec real
(`docs/c6/swagger/agendamento-de-pagamentos.yaml`, adicionada
posteriormente — o esqueleto anterior, sem essa spec, foi descartado e
reescrito do zero).

## Rodando localmente

1. Copie `.env.example` para `.env` e preencha:
   - `DATABASE_URL` apontando para um Postgres local.
   - `C6_CLIENT_ID`/`C6_CLIENT_SECRET` das credenciais de sandbox.
   - `C6_MTLS_CERT_PATH`/`C6_MTLS_KEY_PATH` apontando para os arquivos em
     `docs/c6/auth/` (nunca comitar esses arquivos — já estão no
     `.gitignore`).
   - `C6_WEBHOOK_PATH_SECRET` com um valor aleatório.

2. Exporte as variáveis e rode:

   ```sh
   go run ./cmd/api
   ```

   Migrations em `internal/platform/postgres/migrations/` são aplicadas
   automaticamente no startup.

3. Testes unitários:

   ```sh
   go test ./...
   ```

4. Testes de integração (Bruno): com o servidor rodando (passo 2 ou via
   Docker), veja `tests/bruno/`.

## Endpoints

| Feature | Rotas |
|---|---|
| Checkout/cartão | `POST /v1/checkouts`, `GET /v1/checkouts/{id}`, `PUT /v1/checkouts/{id}/cancel` |
| Assinaturas | `POST /v1/subscriptions/monthly`, `POST /v1/subscriptions/annual-installment`, `PUT /v1/subscriptions/{id}/cancel` |
| Boleto | `POST /v1/bank-slips`, `GET /v1/bank-slips/{id}`, `PUT /v1/bank-slips/{id}`, `PUT /v1/bank-slips/{id}/cancel` |
| PIX | `POST /v1/pix/charges`, `GET /v1/pix/charges/{txid}`, `GET /v1/pix/charges`, `POST /v1/pix/webhook` |
| Webhooks (genérico) | `POST /v1/webhooks/register`, `POST /webhooks/c6/{secret}` (recebimento) |

## Logs

A aplicação loga em JSON (stdout) toda requisição HTTP recebida (método,
path, status, duração) e toda chamada feita ao C6 (método, URL, status,
duração) — em `internal/platform/httpserver/middleware.go` e
`internal/provider/c6/client/client.go`, respectivamente. Para ver também
os corpos (request/response) das chamadas, defina `LOG_LEVEL=debug` e
`LOG_HTTP_BODY=true` no `.env`. Headers nunca são logados, então o
`Authorization`/bearer token e o `client_secret` jamais aparecem nos logs,
independentemente desse flag.

## Testes de integração (Bruno)

Coleção em `tests/bruno/`, no mesmo padrão usado em outros projetos (ex:
`nfs`): `bruno.json`, `environments/*.bru` (vars + `vars:secret`), pastas
por feature com arquivos numerados, blocos `assert`, e
`script:post-response` para encadear requisições (ex: captura `checkout_id`
da criação para usar na consulta/cancelamento).

```sh
cd tests/bruno
npx @usebruno/cli run -r --env local --env-var C6_WEBHOOK_PATH_SECRET=<valor-do-.env>
```

Ou use o runner que já gera o relatório formatado (ver seção abaixo):

```sh
cd tests/bruno
C6_WEBHOOK_PATH_SECRET=<valor-do-.env> ./run.sh
```

- `health/`: healthcheck.
- `checkouts/`: itens 9.a/9.b/9.c, 10.a, 11.a, 12.a do roteiro — crédito,
  débito, autenticação opcional, consulta, cancelamento, tokenização
  (`save_card`), validações.
- `subscriptions/`: assinatura mensal, parcelamento anual, cancelamento e
  validações (não é um produto do roteiro — orquestração nossa sobre
  checkout, ver "Status conhecido" abaixo).
- `boleto/`: itens 1.a/1.b, 2.a/2.b/2.c, 3.a/3.b, 4.a/4.b, 5.a/5.c, 6.a do
  roteiro — emissão (simples, só juros, só multa, multa fixa, desconto
  único e escalonado), validações, consulta, alteração (vencimento e
  valor/multa/juros) e baixa (boleto ativo e no dia do vencimento).
- `pix/`: itens 7.1 a 7.8 do roteiro — cobrança imediata (com/sem devedor),
  cobrança com vencimento, consulta por txid, listagem por período,
  validações, e webhook próprio do produto PIX (item 11, ver aviso abaixo).
- `webhooks/`: item 13.a do roteiro — registro de webhook genérico no C6
  (item 01, ver aviso abaixo) e recebimento de notificação simulada + teste
  de idempotência (totalmente locais, não dependem do C6).

Todas as chamadas de criação/consulta/alteração/cancelamento batem na API
real do C6 sandbox e precisam de credenciais válidas.

**Cobertura do roteiro de homologação** (`docs/c6/homologação/Roteiro
conformidade_Boleto_PIX_Pagamentos_Checkout.docx`):

| Bloco | Cobertura | Observação |
|---|---|---|
| Boleto (itens 1-6) | ✅ completa | item 4.b (baixa de vencido) é uma aproximação: testamos com `due_date` = hoje, já que nossa validação rejeita `due_date` no passado e não há como simular um boleto vencido há dias no sandbox — validar manualmente antes de submeter o roteiro ao C6 |
| PIX (item 7) | ✅ completa | 8/8 |
| DDA/Agendamento (item 8) | ✅ completa | 7/7 testes passando contra o sandbox real (8.1-8.6). Descoberta durante a validação: `POST /decode` é **assíncrono** — retorna o `group_id` na hora, mas o C6 decodifica cada item (integridade do boleto/chave PIX, consulta DICT) em segundo plano; uma consulta imediata a `GET /{group_id}/items` pode retornar `422` ("ainda estão no processo de decodificação"). O adapter (`internal/provider/c6/paymentscheduling_adapter.go`) já trata isso com retry/backoff (até ~30s) antes de propagar erro. |
| Checkout C6 Pay (itens 9-13) | ✅ quase completa | **Atualização 2026-07-07: o Checkout passou a funcionar contra o sandbox real** — 9.a/9.b/9.c/12.a → `201`, 10.a → `200`, 11.a → `204` (8/9 testes passando; a única falha é `09`, que é InfinitePay, não C6). O C6 habilitou o produto Checkout/C6 Pay para a credencial em algum momento entre 22/jun (ainda dava 502/401) e 07/jul. Pendências que não saem só do Bruno: item 12.b (capturar com token — `POST /v1/checkouts/authorize` existe internamente, usado pelo worker de assinaturas, mas não é rota HTTP pública) e item 13 (registrar webhook do checkout — flag `--include-webhook-register`, efeito colateral persistente) |

**Status conhecido por produto**: com as mesmas credenciais, **PIX, Boleto
e Checkout funcionam de ponta a ponta** contra o sandbox real. PIX e Boleto
já funcionavam desde 2026-06-19 (19/19 e 10/10 nos testes). O **Checkout**
retornava `401 Não autorizado` de forma consistente até 22/jun (o produto
Checkout/C6 Pay ainda não estava habilitado para a credencial), mas
**passou a funcionar em 2026-07-07** (9.a/9.b/9.c/12.a → `201`, 10.a →
`200`, 11.a → `204`), indicando que o C6 habilitou o produto para a
credencial nesse intervalo. `subscriptions` dependia dessa mesma
habilitação (toda assinatura cria um checkout internamente na primeira
cobrança), então deve passar a funcionar junto. Sandbox opera das 7h às 23h
(ver `docs/c6/auth/email_c6.txt`).

Também observado: alterar ou baixar um boleto **imediatamente** após
emiti-lo pode retornar `400` com `"Evento não pode ser realizado, pois já
existe uma requisição à CIP sujeita a aprovação"` — é uma regra de negócio
real do C6 (o registro inicial na CIP ainda está pendente de confirmação),
não um bug. Os testes `boleto/07` e `boleto/09` toleram esse desfecho via
bloco `tests{}`; `boleto/10`, `11`, `17` e `19` esperam
`cip_settle_wait_ms` (90s por padrão, configurável nos arquivos de
ambiente) antes de tentar, e devem ter sucesso real (`200`/`204`).

### Relatório de execução

`tests/bruno/run.sh` roda a suíte (pulando por padrão o registro de webhook,
que tem efeito colateral persistente na conta) e grava um resumo em
Markdown em `tests/bruno/reports/latest.md` — status geral, contagem de
sucesso/falha por pasta, e o detalhe de cada falha (asserção, status HTTP,
corpo da resposta). O JSON bruto fica em `tests/bruno/reports/latest.json`.
Esses arquivos são gerados a cada execução e não são versionados
(`tests/bruno/reports/` está no `.gitignore`).

Para incluir o teste de registro de webhook na execução: `./run.sh --include-webhook-register`.

## Rodando com Docker

A pasta `docker/` contém um Dockerfile para o `banking` (build multi-stage,
Go -> alpine) e um para o `postgres` (wrapper sobre a imagem oficial),
orquestrados por `docker/docker-compose.yml`.

1. Copie **os dois** arquivos de exemplo (são complementares, não
   alternativos):
   - `.env.example` (raiz) → `.env`: segredos do C6 (`C6_CLIENT_ID`,
     `C6_CLIENT_SECRET`, `C6_WEBHOOK_PATH_SECRET`, etc.). O `banking` os
     carrega de lá via `env_file` no compose — é o único lugar onde esses
     valores existem, usado tanto rodando com `go run` quanto via Docker.
   - `docker/.env.example` → `docker/.env`: só o que é específico da
     orquestração — credenciais do Postgres, portas, e `C6_CERT_FILE`/
     `C6_KEY_FILE` apontando (caminho relativo a `docker/`) para os
     arquivos `.crt`/`.key` já presentes em `docs/c6/auth/`. Eles são
     montados como bind mount somente leitura no container, nunca
     copiados para a imagem.

2. Suba o stack:

   ```sh
   cd docker
   docker compose up --build
   ```

   O Postgres sobe com healthcheck (`pg_isready`); o `banking` só inicia
   depois que o Postgres estiver saudável, aplica as migrations
   automaticamente e expõe `GET /healthz` (usado pelo próprio healthcheck
   do container). Portas padrão: `8082` (API) e `7532` (Postgres),
   configuráveis via `HTTP_PORT`/`POSTGRES_PORT` no `docker/.env`.

3. Para derrubar (incluindo o volume de dados do Postgres):

   ```sh
   docker compose down -v
   ```

## Estrutura

- `internal/checkout`, `internal/subscription`, `internal/webhook`,
  `internal/boleto`, `internal/pix`: features de negócio, agnósticas de
  provedor (só conhecem a *port* de cada uma).
- `internal/provider/c6`: adapters concretos do C6 (mTLS, auth, tradução de
  payload unificado <-> DTO do C6).
- `internal/platform`: infraestrutura compartilhada (config, Postgres,
  registry de providers, cron worker, logging).
- `internal/paymentscheduling`: esqueleto (DDA/agendamento) — bloqueado por
  falta de spec do C6, ver seção Status.
- `docker/`: Dockerfiles (`docker/banking`, `docker/postgres`) e
  `docker-compose.yml` para rodar o stack completo localmente.
- `tests/bruno/`: coleção de testes de integração via Bruno.
