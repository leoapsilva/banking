# Plano — endpoint de atualização de cupom

> **Motivador concreto**: em 15/08/2026 foi decidido dar prazo de validade ao cupom `LANCAMENTO50`
> (50% off, sem `valid_until`, criado no lançamento). O `POST /v1/coupons` cria cupom com
> `valid_until`, mas **não existe nada que atualize um cupom já criado** — a única saída é `UPDATE`
> manual no Postgres de produção. Este documento planeja o endpoint que elimina essa necessidade.

📌 **Numeração**: sem número de release atribuído. `docs/project/RELEASE_0.1.0.md` é a única release
numerada do repo e trata do checkout InfinitePay; quem prioriza decide onde isto entra.

---

## O problema

`billing_coupons` (migration `0006_billing_plans_coupons.sql`) tem `valid_from`, `valid_until` e
`max_redemptions` — os três campos que controlam **disponibilidade** de um cupom. Nenhum deles é
alcançável depois da criação:

| Operação | Existe? |
|---|---|
| `GET /v1/coupons/{code}/validate` | sim |
| `POST /v1/coupons` | sim (desde ago/2026) |
| **atualizar cupom** | **não** |
| listar cupons | não |
| remover cupom | não |

Isso obriga a `UPDATE` manual em produção para qualquer ajuste de campanha, que é exatamente a
pendência já registrada do lado do Cores (*"parâmetro comercial só alterável por SQL manual"*,
`docs/RELEASE_PRIORIDADES_2026-07.md`, a ser resolvida pelo backoffice).

## Escopo proposto

### `PATCH /v1/coupons/{code}`

`PATCH` e não `PUT`: a intenção real é sempre alterar um ou dois campos, e um `PUT` obrigaria o
chamador a reenviar o cupom inteiro — com risco de zerar sem querer o que ele não conhece.

**Editável** — controla quando e quanto o cupom pode ser usado:

| Campo | Regra |
|---|---|
| `valid_from` | RFC3339 ou `null` (remove o limite inferior) |
| `valid_until` | RFC3339 ou `null` (torna o cupom perpétuo). Data no passado é **permitida** — é assim que se encerra uma campanha imediatamente |
| `max_redemptions` | inteiro > 0 ou `null` (ilimitado). Não pode ser menor que `redemptions_count` atual |

**Não editável** — define o que foi prometido a quem já recebeu o código:

| Campo | Por que fica de fora |
|---|---|
| `code` | é a chave que já circula em emails e links |
| `percent_off` / `amount_off_cents` | mudar o desconto de um cupom em circulação altera retroativamente uma oferta já comunicada. Campanha nova = cupom novo |
| `duration` | idem — muda o valor do que foi oferecido |
| `applicable_frequencies` | hoje só existe plano anual em uso; abrir isso sem necessidade real amplia a superfície de erro |
| `redemptions_count` | é contador, não configuração |

> A v0.19.6 do Cores (parâmetros comerciais no backoffice) quer editar o **percentual do cupom de
> boas-vindas**. Não conflita: lá o que muda é o parâmetro que gera cupons **novos**
> (`app_settings`, no Cores), não o desconto de um cupom que já está na mão de alguém.

### Ausente ≠ nulo

A distinção é o ponto delicado do PATCH: `{"valid_until": null}` significa *"remova o prazo"*, e
omitir a chave significa *"não mexa"*. Um `*string` sozinho não separa os dois casos — nos dois ele
chega `nil`.

Solução: decodificar o corpo **duas vezes** — uma em `map[string]json.RawMessage`, só para saber
quais chaves vieram, e outra na struct tipada, para os valores. O service recebe então um request
com campos `*(*time.Time)` ou, mais simples de ler, um par `{Value, Present}` por campo.

### Respostas

| Situação | Status |
|---|---|
| atualizado | `200` com a representação do cupom |
| corpo inválido / data fora do RFC3339 | `400` |
| `valid_until` anterior a `valid_from` | `400` |
| `max_redemptions` menor que `redemptions_count` | `400` |
| nenhum campo editável no corpo | `400` |
| código inexistente | `404` |
| tentativa de alterar campo imutável | `400`, nomeando o campo — silenciar seria pior |

## Camadas a tocar

| Camada | Mudança |
|---|---|
| `internal/billing/handler/coupon_handler.go` | `updateCoupon`, no padrão do `createCoupon` (mesmo `parseOptionalRFC3339`, mesmo `writeError`) |
| `internal/billing/handler/handler.go` | `mux.HandleFunc("PATCH /v1/coupons/{code}", h.updateCoupon)` |
| `internal/billing/service` | `UpdateCoupon`, com a validação de faixa; reaproveita `ErrInvalidCouponRequest` |
| `internal/billing/domain` | `ErrCouponNotFound` já existe e serve |
| `internal/billing/repository/pricing.go` | `UPDATE billing_coupons SET ... WHERE code = $n` montado só com as colunas presentes, com `RETURNING` |

Sem migration: as três colunas já existem.

## Segurança

O endpoint fica sob a mesma `X-API-Key` de todo o resto — **não há tier de admin** neste serviço
hoje, e a única chave em uso é a do Cores. Isso significa que a mesma credencial que cria assinatura
passa a poder encerrar campanha. É aceitável enquanto só o Cores tiver chave, mas deixa de ser no
momento em que existir um segundo consumidor; registrar como limitação conhecida em vez de
descobrir depois.

Não há trilha de auditoria no repo — nenhuma tabela registra quem mudou o quê. No mínimo, logar a
alteração com o código do cupom e os campos afetados.

## Testes

| # | Tipo | Cenário | Esperado |
|---|---|---|---|
| 1 | unitário | `valid_until` para data futura | `200`, coluna atualizada |
| 2 | unitário | `valid_until: null` em cupom com prazo | `200`, coluna vira `NULL` |
| 3 | unitário | corpo sem nenhuma chave editável | `400` |
| 4 | unitário | `valid_until` anterior a `valid_from` | `400`, nada gravado |
| 5 | unitário | `max_redemptions` abaixo de `redemptions_count` | `400` |
| 6 | unitário | tentativa de alterar `percent_off` | `400` nomeando o campo |
| 7 | unitário | código inexistente | `404` |
| 8 | integração | `valid_until` no passado → `ResolvePrice` passa a devolver `ErrCouponExpired` | cupom recusado no checkout seguinte |
| 9 | integração | assinatura **já criada** com o cupom | preço preservado — a validade é avaliada na resolução de preço, não retroativamente |

## Fora de escopo

- **Listar cupons** (`GET /v1/coupons`) — útil para o backoffice do Cores, mas é outra história de
  usuário; entra quando houver tela que consuma.
- **Excluir cupom** — encerrar por `valid_until` é melhor: preserva histórico e o
  `redemptions_count` de quem já usou.
- **Tier de admin na autenticação** — problema do serviço inteiro, não deste endpoint.
- **Trilha de auditoria** — idem; aqui só entra o log.
