# Guia de Integração — Checkout & Assinaturas

Este guia descreve como uma aplicação backend consome a API do banking para criar
cobranças únicas e assinaturas. Para a spec completa de campos e tipos veja
`openapi.yaml` neste mesmo diretório.

---

## Sumário

1. [Visão geral](#visão-geral)
2. [Comparativo de provedores](#comparativo-de-provedores)
3. [Fluxo de checkout único](#fluxo-de-checkout-único)
4. [Consulta de status e polling](#consulta-de-status-e-polling)
5. [Fluxo de assinaturas](#fluxo-de-assinaturas)
6. [Tabela de transições de status](#tabela-de-transições-de-status)
7. [Configuração do banking (ops)](#configuração-do-banking-ops)
8. [Erros comuns](#erros-comuns)

---

## Visão geral

O banking expõe uma API única para múltiplos provedores de pagamento. Você
escolhe o provedor via o campo `baas` em cada request. O banking cuida de
autenticação, mapeamento de payload e persistência — você só recebe URLs e IDs.

```
sua app  →  POST /v1/checkouts  →  banking  →  provedor (C6 / InfinitePay)
                                                     ↓ webhook de pagamento
sua app  ←  GET /v1/checkouts/{id}  ←  banking (DB atualizado pelo webhook)
```

---

## Comparativo de provedores

| Característica | `baas: c6` | `baas: infinitepay` |
|---|---|---|
| Tipo de página de pagamento | Transparente (card hash do frontend) | Hospedada (InfinitePay cuida do checkout) |
| Formas de pagamento | Crédito / Débito | Pix (grátis) + Cartão até 12x (comprador escolhe) |
| GET de status em tempo real | Sim (chama a API C6) | Não — leitura local (DB-first) |
| Cartão tokenizado / recorrência | Sim (`save_card`, `token`) | Não suportado |
| Cancelamento via API | Sim | Não (status local marcado como CANCELLED) |
| Assinatura mensal automática | Via token salvo + cron interno | Via plano pré-criado no app InfinitePay |
| Dados de pagamento pós-aprovação | Via webhook C6 | Via webhook InfinitePay |

---

## Fluxo de checkout único

### InfinitePay

```
1. POST /v1/checkouts
   { "baas": "infinitepay", "amount_cents": 15000, "currency": "BRL",
     "description": "Produto X", "redirect_url": "https://seusite.com/ok",
     "payer": { "name": "Maria Souza", "email": "maria@email.com" } }

   ← 201  { "id": "abc123slug", "url": "https://invoice.infinitepay.io/..." }

2. Redirecione o comprador para url
   O comprador paga na página da InfinitePay (Pix, cartão, etc.)

3. O comprador volta para redirect_url (opcional — pode fechar a aba)

4. Polling: GET /v1/checkouts/abc123slug?baas=infinitepay
   ← { "status": "PAID", "capture_method": "pix", "paid_amount_cents": 15000, ... }
```

**Importante:** `GET /v1/checkouts/{id}` para `infinitepay` lê o banco local,
que é atualizado de forma assíncrona quando o banking recebe o webhook da
InfinitePay. Não há chamada à API da InfinitePay neste GET.

### C6

```
1. Frontend captura cartão → obtém card_hash via JS da C6

2. POST /v1/checkouts
   { "baas": "c6", "amount_cents": 15000, "currency": "BRL",
     "payer": { "name": "João", "tax_id": "12345678909", "email": "j@e.com" },
     "card": { "type": "CREDIT", "installments": 1, "capture": true,
               "authenticate": "NOT_REQUIRED",
               "card_info": { "card_hash": "<hash-do-frontend>" } } }

   ← 201  { "id": "c6-checkout-uuid", "url": "https://..." }

3. GET /v1/checkouts/c6-checkout-uuid?baas=c6   (chama a API C6 em tempo real)
   ← { "status": "PAID", ... }
```

---

## Consulta de status e polling

### Quando fazer polling

- **InfinitePay**: o comprador acabou de voltar pelo `redirect_url` e o webhook
  pode não ter chegado ao banking ainda. Tente no máximo 3–5 vezes com 2–3 s
  de intervalo antes de mostrar "aguardando confirmação".

- **C6**: polling é a forma principal de rastrear status, pois cada GET chama a
  API C6 em tempo real.

### Status PAID — campos extras disponíveis

Quando `status == "PAID"`, o response de `GET /v1/checkouts/{id}` inclui:

```json
{
  "status": "PAID",
  "paid_amount_cents": 15600,
  "capture_method": "credit_card",
  "installments": 3,
  "receipt_url": "https://comprovante.../abc123",
  "transaction_id": "txn-0001",
  "paid_at": "2026-07-06T14:32:10Z"
}
```

`paid_amount_cents` pode ser maior que `amount_cents` quando há juros de
parcelamento e a operadora repassa esse custo ao comprador.

---

## Fluxo de assinaturas

### Plano mensal InfinitePay (via link pré-criado)

A InfinitePay gerencia as cobranças mensais automáticas pelo painel deles. Você
só precisa redirecionar o comprador para se inscrever:

```
GET /v1/subscriptions/infinitepay/plans/monthly
← { "url": "https://invoice.infinitepay.io/plans/seu-handle/CODIGO" }

Redirecione o comprador para url
```

O banking devolve a URL estática configurada pela operação em
`INFINITEPAY_PLAN_MONTHLY_URL`. Não há registro de `subscription_id` neste fluxo.

### Plano anual InfinitePay (cobrança única via link)

```
POST /v1/subscriptions/annual
{ "baas": "infinitepay", "customer_name": "Maria", "customer_email": "m@e.com",
  "amount_cents": 150000, "currency": "BRL",
  "redirect_url": "https://seusite.com/assinatura/ok" }

← 201
{ "subscription_id": "uuid-da-sub",
  "checkout_id": "abc123slug",
  "checkout_url": "https://invoice.infinitepay.io/..." }
```

- Guarde `subscription_id` para cancelamento.
- Redirecione o comprador para `checkout_url`.
- Acompanhe o pagamento via `GET /v1/checkouts/{checkout_id}?baas=infinitepay`.

### Plano anual C6 parcelado

```
POST /v1/subscriptions/annual-installment
{ "baas": "c6", "customer_name": "João", "customer_tax_id": "12345678909",
  "customer_email": "j@e.com", "amount_cents": 150000, "currency": "BRL",
  "installments": 12,
  "redirect_url": "https://seusite.com/assinatura/ok" }

← 201  { "subscription_id": "...", "checkout_id": "...", "checkout_url": "..." }
```

Juros de parcelamento ficam com o comprador (passados pela operadora).

### Plano mensal C6 com cartão salvo

```
POST /v1/subscriptions/monthly
{ "baas": "c6", "customer_name": "João", ..., "amount_cents": 9900 }

← 201  { "subscription_id": "...", "checkout_id": "...", "checkout_url": "..." }
```

Após o primeiro pagamento com `save_card=true`, o banking salva o token e cobra
automaticamente a cada mês sem nova interação do comprador.

### Cancelar assinatura

```
PUT /v1/subscriptions/{subscription_id}/cancel
← 204
```

Para assinaturas InfinitePay: apenas marca como CANCELLED localmente (InfinitePay
não oferece cancelamento de link via API).

---

## Tabela de transições de status

| Status | Significado |
|---|---|
| `CREATED` | Checkout criado, aguardando o comprador pagar |
| `IN_PROGRESS` | Processamento em andamento (C6) |
| `PAID` | Pagamento confirmado |
| `DECLINED` | Recusado pela operadora (C6) |
| `EXPIRED` | Link expirou sem pagamento |
| `CANCELLED` | Cancelado pela sua aplicação |
| `ERROR` | Erro interno durante o processamento |

---

## Configuração do banking (ops)

Variáveis de ambiente necessárias para os endpoints InfinitePay funcionarem:

| Variável | Exemplo | Descrição |
|---|---|---|
| `INFINITEPAY_BASE_URL` | `https://api.checkout.infinitepay.io` | Endpoint da API InfinitePay |
| `INFINITEPAY_HANDLE` | `meuhandle` | InfiniteTag sem `$` |
| `INFINITEPAY_WEBHOOK_URL` | `https://banking.interno/webhooks/infinitepay/{secret}` | URL que a InfinitePay chamará ao aprovar pagamento |
| `INFINITEPAY_WEBHOOK_PATH_SECRET` | `(gerado)` | Segmento secreto na URL do webhook |
| `INFINITEPAY_PLAN_MONTHLY_URL` | `https://invoice.infinitepay.io/plans/...` | Link do plano mensal pré-criado no app InfinitePay |

O banking **não registra** o endpoint de webhook com a InfinitePay centralmente —
a `webhook_url` é enviada em cada `POST /links`. Portanto `INFINITEPAY_WEBHOOK_URL`
deve ser a URL pública e estável do banking.

Para C6, registre o webhook uma única vez após subir o serviço:

```
POST /v1/webhooks/register
{ "baas": "c6", "service": "CHECKOUT", "url": "https://banking.interno/webhooks/c6/{secret}" }
```

---

## Erros comuns

| Situação | Causa provável |
|---|---|
| `GET /v1/checkouts/{id}?baas=infinitepay` retorna `CREATED` mesmo após o comprador pagar | Webhook da InfinitePay ainda não chegou. Aguarde e repita o polling. |
| `POST /v1/checkouts` retorna `502` para InfinitePay | `INFINITEPAY_HANDLE` inválido ou handle não habilitado para Checkout na conta InfinitePay. |
| `GET /v1/subscriptions/infinitepay/plans/monthly` retorna `503` | `INFINITEPAY_PLAN_MONTHLY_URL` não configurado no banking. |
| `PUT /v1/subscriptions/{id}/cancel` retorna `502` para C6 | Checkout já foi aprovado e a janela de cancelamento C6 expirou. |
| Webhook InfinitePay não é processado | Verificar que `INFINITEPAY_WEBHOOK_PATH_SECRET` no banking corresponde ao segmento na URL registrada em `INFINITEPAY_WEBHOOK_URL`. |
