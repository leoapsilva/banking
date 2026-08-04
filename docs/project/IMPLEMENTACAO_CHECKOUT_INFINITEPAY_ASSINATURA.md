# Guia de implementação — Checkout InfinitePay para assinatura de app (mensal/anual)

> Documento destinado a orientar a implementação (via Claude Code) da integração de checkout
> InfinitePay para o caso de uso: um site vende uma assinatura de app, mensal ou anual, usando o
> banking como intermediário de pagamento. Complementa
> [RELEASE_0.1.0.md](RELEASE_0.1.0.md) (arquitetura geral do provider InfinitePay) — leia aquele
> documento primeiro para o desenho do adapter/domínio/webhook. Este documento foca no caso de uso
> de assinatura e no contrato de retorno ao backend do site.

## Contexto: por que o módulo `subscription` atual não serve de exemplo direto

`internal/subscription/service/service.go` já implementa assinatura mensal e anual, mas **inteiro
desenhado em torno do modelo C6**: a assinatura mensal funciona criando um checkout inicial com
`SaveCard: true, Recurrent: true`, guardando o `SavedCardToken` devolvido no webhook, e cobrando os
ciclos seguintes via `checkoutSvc.Authorize()` com esse token (`chargeOne`, linha ~298-350).

A InfinitePay **não suporta nada disso**: não tokeniza cartão, não tem `Authorize` (cobrança direta
sem checkout hospedado) — ver decisão em [RELEASE_0.1.0.md](RELEASE_0.1.0.md) seção "Mudanças > 2",
`Authorize()` do adapter InfinitePay retorna `ErrNotSupported`. Isso significa que **o modelo de
assinatura mensal recorrente via token não se aplica à InfinitePay** e precisa de um fluxo próprio.

## Modelo de assinatura para InfinitePay

### Anual (pagamento único, com opção de parcelamento pelo comprador)
Um único `POST /v1/checkouts` com `"baas": "infinitepay"` e o valor total da assinatura anual.
O comprador escolhe a forma de pagamento na página hospedada da InfinitePay:
- **Pix** — taxa zero, sem parcelamento.
- **Cartão de crédito** — o comprador escolhe o número de parcelas (até 12x). O banking **não
  dita** o número de parcelas; quem decide é o comprador na página. O que volta no webhook
  (`installments`, `capture_method`) é informativo, não algo configurado no request.
- **Apple Pay / Google Pay** — também suportados na página hospedada.

O usuário confirmou que a assinatura anual criada no produto de Planos da InfinitePay cobra em 1x
apenas, independente do período. Usar o checkout via API (`POST /links`) é o caminho para oferecer
parcelamento na assinatura anual, já que aí o comprador escolhe livremente até 12x no cartão.

### Mensal (link de plano pré-criado)
A InfinitePay tem um produto de **Planos e Assinaturas** que gerencia cobranças recorrentes
automáticas (lembra o cliente via WhatsApp, cobra o cartão todo mês), mas ele é **exclusivamente
criado via UI** (app InfinitePay ou Conta Web) — sem API para criar planos programaticamente.

O modelo adotado: o plano mensal é criado **uma única vez no app da InfinitePay**, seu link é
armazenado como **variável de ambiente** (`INFINITEPAY_PLAN_MONTHLY_URL`) e o banking simplesmente
o devolve quando o site pede. Nenhuma chamada à API da InfinitePay é feita no momento da requisição.

```bash
# .env.example — junto com as demais vars InfinitePay
INFINITEPAY_PLAN_MONTHLY_URL=https://invoice.infinitepay.io/plans/seu-handle/CODIGO
```

Endpoint novo no handler de subscriptions (`internal/subscription/handler/handler.go`):
```
GET /v1/subscriptions/infinitepay/plans/monthly
→ 200 { "url": "https://invoice.infinitepay.io/plans/..." }
→ 503 { "error": "monthly plan not configured" }  ← se a env var estiver vazia
```

O handler lê `INFINITEPAY_PLAN_MONTHLY_URL` via config (injetado no construtor do
`subscription.Handler`) — sem camada de serviço ou repositório, já que é config estática.
Se futuramente houver múltiplos planos (tiers), migra para tabela `infinitepay_plans` sem mudar
a interface da rota.

## O que o banking retorna ao backend do site

O backend do site (cliente da nossa API) precisa saber **como o pagamento foi feito** para liberar o
acesso ao app corretamente (ex.: mostrar "pago via Pix" vs "pago em 3x no cartão"). Hoje,
`GET /v1/checkouts/{id}` (`internal/checkout/handler/handler.go`, `checkoutDetailsResponse`) só
devolve `{id, status, amount_cents, currency, saved_card_token}` — **insuficiente** para esse caso de
uso. Não tem `capture_method`, `installments`, `paid_amount`, `receipt_url`, `transaction_nsu`, nem
data de pagamento.

### Extensão necessária ao domínio e à resposta

- `domain.CheckoutDetails` (em `internal/checkout/domain/checkout.go`) ganha campos genéricos
  (preenchíveis por qualquer provider, não exclusivos da InfinitePay):
  ```go
  type CheckoutDetails struct {
      ProviderCheckoutID string
      Status             Status
      Amount             Money
      SavedCardToken     *string
      // Novo:
      PaidAmount         *Money     // valor efetivamente pago (pode != Amount, ex. juros de parcelamento)
      CaptureMethod      *string    // "pix" | "credit_card" | "debit_card" | "boleto"
      Installments       *int
      ReceiptURL          *string
      TransactionID       *string    // transaction_nsu na InfinitePay; id de transação no C6
      PaidAt              *time.Time
  }
  ```
- O adapter InfinitePay (`checkout_adapter.go`/`webhook_adapter.go`, ver
  [RELEASE_0.1.0.md](RELEASE_0.1.0.md) seção 2-3) popula esses campos a partir do
  `dto.WebhookPayload`/`dto.PaymentCheckResponse` quando o evento chega — e como a estratégia de
  `Get()` para InfinitePay é **DB-first** (não chama a InfinitePay de novo), esses campos também
  precisam ser persistidos em `checkouts` (colunas novas: `capture_method`, `installments`,
  `paid_amount_cents`, `receipt_url`, `transaction_id`, `paid_at`) e lidos de lá tanto para C6 quanto
  para InfinitePay — campo genérico do módulo, não exclusivo de um provider.
- `checkoutDetailsResponse` (handler) ganha os mesmos campos em JSON (`snake_case`), todos
  `omitempty`, já que nem todo provider/status os preenche (ex.: antes do pagamento, ficam nulos).

### Exemplo de resposta após pagamento confirmado

```json
{
  "id": "abc123",
  "status": "PAID",
  "amount_cents": 150000,
  "currency": "BRL",
  "paid_amount_cents": 153000,
  "capture_method": "credit_card",
  "installments": 3,
  "receipt_url": "https://comprovante.infinitepay.io/abc123",
  "transaction_id": "9f3e1c2a-...",
  "paid_at": "2026-06-22T14:32:10Z"
}
```

O backend do site usa esse retorno para, por exemplo, exibir "Assinatura anual — pago em 3x no
cartão" e liberar o acesso ao app a partir de `paid_at`.

## Como funciona a `redirect_url` e o que ela pode apresentar

`redirect_url` é configurada pelo backend do site no `POST /v1/checkouts` (campo já existente em
`domain.CreateCheckoutRequest.RedirectURL`) e é para onde o **navegador do comprador** é levado
depois de concluir (ou tentar concluir) o pagamento na página hospedada da InfinitePay — não é uma
chamada de servidor, é um redirecionamento de browser (ver
[CONCEITOS_WEBHOOK_REDIRECT.md](CONCEITOS_WEBHOOK_REDIRECT.md)).

A InfinitePay acrescenta parâmetros de query nessa URL: `order_nsu`, `slug`, `transaction_nsu`,
`capture_method`, `receipt_url`. Exemplo:
```
https://seusite.com/assinatura/concluida?order_nsu=ASSIN-2026-06&slug=abc123&transaction_nsu=9f3e1c2a&capture_method=credit_card&receipt_url=https%3A%2F%2Fcomprovante.infinitepay.io%2Fabc123
```

### O que a página de retorno do site pode (e deve) fazer

1. **Nunca confiar só na query string** para liberar acesso ao app — o comprador pode editar a URL
   manualmente, ou a aba pode ter sido fechada antes do redirect acontecer de verdade. A página deve
   usar `slug` (ou o `order_nsu` que o próprio site gerou) para chamar
   `GET /v1/checkouts/{slug}?baas=infinitepay` no backend do site → banking, e **só então** confiar
   no `status` retornado (a fonte de verdade real é o webhook já processado, lido do banco — ver
   seção anterior).
2. Com a resposta confirmada do banking, a página pode apresentar, por exemplo:
   - Pago via **Pix**: "Pagamento confirmado via Pix! Sua assinatura está ativa." + link para
     `receipt_url`.
   - Pago via **cartão em N parcelas**: "Pagamento confirmado — 3x no cartão de crédito. Sua
     assinatura anual está ativa até DD/MM/AAAA." (data calculada pelo site a partir de `paid_at` +
     período da assinatura).
   - Pagamento **ainda não confirmado** (`status != PAID` quando a página carrega, porque o webhook
     ainda não chegou): "Estamos confirmando seu pagamento, isso pode levar alguns segundos" — a
     página pode fazer polling curto (poucas tentativas, alguns segundos de intervalo) em
     `GET /v1/checkouts/{slug}` até virar `PAID`, ou simplesmente orientar o usuário a checar o
     e-mail/app depois.
   - **Sem `redirect_url` configurada**: a InfinitePay redireciona automaticamente para a página de
     comprovante deles após ~3 segundos (comportamento da própria InfinitePay, fora do nosso
     controle) — então toda integração de assinatura deve configurar `redirect_url` para manter o
     comprador dentro do fluxo do site.

## Rotas novas em `internal/subscription/handler/handler.go`

O handler de subscriptions já tem: `POST /v1/subscriptions/monthly` (C6), `POST
/v1/subscriptions/annual-installment` (C6), `PUT /v1/subscriptions/{id}/cancel`. Adicionar:

| Rota | Método | BaaS | O que faz |
|---|---|---|---|
| `POST /v1/subscriptions/annual` | InfinitePay | cria checkout via `POST /links` (sem campo `installments` — buyer decide na página) |
| `GET /v1/subscriptions/infinitepay/plans/monthly` | — | lê `INFINITEPAY_PLAN_MONTHLY_URL` da config e devolve |

`POST /v1/subscriptions/annual` é diferente do já existente `annual-installment` (C6): não recebe
`installments` no payload (InfinitePay não aceita esse parâmetro via API) e não chama
`CreateAnnualInstallment` do service (que valida `installments >= 2`). Precisa de um novo método no
subscription service: `CreateAnnualCheckout(ctx, req)` que chama `checkoutSvc.CreateCheckout` com
os campos adequados — sem `Card`, sem `Installments`, sem `SaveCard`.

## Arquivos a tocar (além dos já listados em RELEASE_0.1.0.md)

- `internal/checkout/domain/checkout.go` — novos campos em `CheckoutDetails`.
- `internal/checkout/repository/repository.go` + migration — novas colunas (`capture_method`,
  `installments`, `paid_amount_cents`, `receipt_url`, `transaction_id`, `paid_at`).
- `internal/checkout/handler/handler.go` — `checkoutDetailsResponse` com os novos campos.
- `internal/provider/infinitepay/webhook_adapter.go` — popular esses campos a partir do
  `dto.WebhookPayload` ao montar o `InboundEvent`/ao persistir via `checkoutSink`.
- `internal/subscription/service/service.go` — novo método `CreateAnnualCheckout` (sem
  `installments`, sem `Card`, sem `SaveCard`) para o fluxo InfinitePay anual.
- `internal/subscription/handler/handler.go` — duas novas rotas:
  `POST /v1/subscriptions/annual` e `GET /v1/subscriptions/infinitepay/plans/monthly`.
- `internal/platform/config/` — ler `INFINITEPAY_PLAN_MONTHLY_URL`.
- `.env.example`, `docker/.env.example` — adicionar `INFINITEPAY_PLAN_MONTHLY_URL`.
- `cmd/api/main.go` — injetar `INFINITEPAY_PLAN_MONTHLY_URL` no construtor do
  `subscription.Handler`.

## Verificação

1. `GET /v1/subscriptions/infinitepay/plans/monthly` retorna `{"url": "https://invoice.infinitepay.io/plans/..."}` quando `INFINITEPAY_PLAN_MONTHLY_URL` está configurada; retorna `503` quando vazia.
2. `POST /v1/subscriptions/annual` com `baas=infinitepay` retorna `{subscription_id, checkout_id, checkout_url}` sem precisar de campo `installments` no payload.
3. Pagar o link anual gerado (conta real) e confirmar que `GET /v1/checkouts/{id}` retorna
   `capture_method`, `installments`, `paid_amount_cents`, `receipt_url`, `transaction_id`, `paid_at`
   preenchidos após o webhook ser processado.
4. Configurar `redirect_url` e confirmar que os parâmetros de query (`order_nsu`, `slug`,
   `transaction_nsu`, `capture_method`, `receipt_url`) chegam corretamente.
5. Testar "comprador fecha a aba antes do redirect": status ainda assim atualizado para `PAID` via
   webhook, independente do redirect ter ocorrido.
