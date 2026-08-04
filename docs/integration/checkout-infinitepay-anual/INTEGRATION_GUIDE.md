# Guia de integração — assinaturas InfinitePay via banking

> Para o time/agente que implementa o **backend do site** (cliente da API do banking) para vender
> assinaturas de app via InfinitePay. Use junto com [`openapi.yaml`](openapi.yaml).

## ⚠️ Mudanças de contrato (ago/2026)

**1. Autenticação obrigatória.** Toda chamada precisa do header `X-API-Key`. Sem ele: `401`.
Apenas `/healthz` e `/webhooks/*` são isentos. Peça sua chave ao time do banking.

**2. Você não envia mais o valor.** O endpoint preferencial passa a ser `POST /v1/subscriptions`
com `plan` + `coupon_code` opcional; o banking resolve o preço a partir do catálogo. Isso existe
para que "quanto este cliente paga" tenha uma resposta só, no banking — antes o valor vinha do
cliente e o ciclo ficava aqui, então a pergunta precisava de dois sistemas para ser respondida.

**3. `GET /v1/subscriptions/{id}` passou a existir.** É como você acompanha o estado da assinatura
sem manter uma máquina de estados própria.

Os endpoints antigos que recebem `amount_cents` (`/v1/subscriptions/annual`, `/monthly`,
`/annual-installment`) seguem funcionando para os fluxos C6 existentes, mas **não use para
integrações novas**.

## Os dois planos e seus fluxos

| Plano | Como funciona | Quem gerencia a recorrência |
|---|---|---|
| **Mensal** | Banking devolve link de plano pré-criado no InfinitePay app | InfinitePay (cobra automaticamente, lembra via WhatsApp) |
| **Anual** | Banking cria um checkout novo a cada compra via API | Uma cobrança única; comprador escolhe Pix ou cartão (até 12x) |

---

## Plano Mensal

### Fluxo completo

```
seu backend                    banking
    |                             |
    |--- GET /v1/subscriptions/infinitepay/plans/monthly -->|
    |<-- { url } -----------------|
    |
    |--- redireciona o comprador para url ----------------> InfinitePay
                                                            (inscrição e cobranças automáticas
                                                             gerenciadas pela InfinitePay)
```

### Chamada

```http
GET /v1/subscriptions/infinitepay/plans/monthly
```

Resposta:
```json
{ "url": "https://invoice.infinitepay.io/plans/seu-handle/CODIGO" }
```

Redirecione o comprador para `url`. A InfinitePay cuida da inscrição, armazena o cartão e realiza
as cobranças mensais automaticamente, enviando lembretes por WhatsApp. O site **não precisa fazer
nada mais** — o controle da assinatura mensal fica inteiramente na InfinitePay.

> Se a resposta for `503`, o banking não tem o link configurado — confirme com o time do banking
> que `INFINITEPAY_PLAN_MONTHLY_URL` está preenchida no ambiente.

---

## Plano Anual

### Visão geral do fluxo

```
seu backend                    banking                       InfinitePay              navegador do comprador
    |                             |                                |                          |
    |--- 1. POST /v1/checkouts -->|                                |                          |
    |                             |--- POST /links --------------->|                          |
    |                             |<-- { url, slug } ---------------|                          |
    |<-- { id, url } -------------|                                |                          |
    |                                                                                          |
    |--- 2. redireciona o navegador do comprador para `url` ------------------------------------>|
    |                                                                |<--- comprador paga ------|
    |                             |<---------- 4. webhook (assíncrono, server-to-server) --------|
    |                             |   banking atualiza status no próprio banco
    |                                                                |---- 3. redirect_url + query params ------->|
    |<--- 5. GET /v1/checkouts/{id}?baas=infinitepay (confirma status real) -----------|
```

Note que os passos 3 (redirect) e 4 (webhook) acontecem em paralelo e **não há garantia de ordem** —
o webhook pode chegar antes ou depois do navegador completar o redirect. Por isso o passo 5 (consulta
ao banking) é sempre necessário antes de liberar acesso ao plano, independente do que a query string
do redirect diga.

## Passo 1 — Criar a assinatura anual

```http
POST /v1/subscriptions
Content-Type: application/json
X-API-Key: sua-chave

{
  "baas": "infinitepay",
  "customer_name": "Maria Souza",
  "customer_email": "maria@email.com",
  "plan": "cores-annual",
  "coupon_code": "LANCAMENTO50",
  "redirect_url": "https://seusite.com/assinatura/concluida"
}
```

Resposta `201`:
```json
{
  "subscription_id": "550e8400-...",
  "checkout_id": "abc123",
  "checkout_url": "https://checkout.infinitepay.com.br/abc123",
  "amount_cents": 14995,
  "list_amount_cents": 29990,
  "currency": "BRL",
  "discount_applied": "50% de desconto (ONCE)"
}
```

- **Não envie valor.** O preço vem do plano cadastrado no banking. `amount_cents` na resposta é o
  que será cobrado; `list_amount_cents` é o preço cheio — use os dois para exibir "de X por Y".
- `coupon_code` é opcional. Cupom inválido, expirado, esgotado ou não aplicável ao plano →
  `422` com a razão. Confirme antes da compra com
  `GET /v1/coupons/{code}/validate?plan=cores-annual`.
- Um cupom `ONCE` vale só no primeiro ciclo — a renovação volta ao preço cheio.

> Se `plan` não existir ou estiver inativo, a resposta é `422`. Os planos são cadastrados por
> ambiente; confirme o código com o time do banking.

Pontos de atenção:
- Não envie `installments` — o comprador decide quantas parcelas quer (até 12x) na página da InfinitePay.
- `redirect_url`: **sempre configure**. Sem ela, a InfinitePay redireciona para a própria página de
  comprovante deles após ~3 segundos — ruim para quem está comprando uma assinatura do seu app.
- Resposta:
  ```json
  {
    "subscription_id": "550e8400-...",
    "checkout_id": "abc123",
    "checkout_url": "https://checkout.infinitepay.com.br/abc123"
  }
  ```
  Guarde `subscription_id` (para cancelamento futuro) e `checkout_id` (para consultar o status).

## Passo 2 — Redirecionar o comprador

Redirecione o navegador do comprador (HTTP redirect normal, `302`/`window.location`, etc.) para o
campo `checkout_url` da resposta do passo 1. A partir daí, o comprador interage só com a página hospedada
da InfinitePay e escolhe como pagar — tudo fora do seu site:

| Forma de pagamento | Taxa | Parcelamento |
|---|---|---|
| Pix | Zero | — |
| Cartão de crédito | Conforme tabela InfinitePay | Até 12x — **comprador escolhe** na página |
| Apple Pay / Google Pay | Conforme tabela InfinitePay | Depende do cartão vinculado |

O número de parcelas não é configurável pelo merchant via API — é escolha do comprador. O banking
recebe `installments` de volta no webhook/`GET` para informar ao site quantas parcelas foram
escolhidas.

## Passo 3 — Receber o comprador de volta (`redirect_url`)

Quando o comprador termina (ou tenta terminar) o pagamento, ele é levado de volta para a
`redirect_url` que você configurou, com parâmetros de query anexados pela InfinitePay:

```
https://seusite.com/assinatura/concluida?order_nsu=...&slug=abc123&transaction_nsu=...&capture_method=credit_card&receipt_url=https%3A%2F%2F...
```

**Não confie nesses parâmetros para liberar acesso.** Eles são úteis só para UX imediata (ex.:
mostrar "Pagamento confirmado!" sem esperar) e para saber qual `id`/`slug` consultar no próximo
passo — mas o comprador pode editar a URL manualmente, e a aba pode ter sido fechada antes do
redirect ocorrer de verdade. Trate-os como uma dica, não como prova.

## Passo 4 — Confirmar o status real

Use o `slug` (= `id` retornado no passo 1) para perguntar ao banking qual é o status confirmado:

```http
GET /v1/checkouts/abc123?baas=infinitepay
```

Resposta possível (pagamento já confirmado):
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

Esse `GET` é servido a partir do banco de dados do banking (atualizado pelo webhook que a
InfinitePay envia ao banking de forma assíncrona) — **nunca chama a InfinitePay novamente**. Por
isso:
- Pode ser chamado imediatamente quando o comprador chega na `redirect_url`, sem custo de uma
  chamada externa lenta.
- Se o webhook ainda não chegou (raro, mas possível — é assíncrono), o status ainda aparece como
  `CREATED`/`IN_PROGRESS`. Faça um polling curto (ex.: a cada 2 segundos, até 5-6 tentativas) antes
  de desistir e orientar o comprador a checar e-mail/app depois.
- `status` nunca fica "preso" em erro por falha de rede passageira — é uma leitura local, não uma
  chamada externa que pode falhar.

### Tabela de status

| `status` | Significado | Ação no seu backend |
|---|---|---|
| `CREATED` | Link gerado, comprador ainda não pagou (ou webhook não chegou ainda) | Aguardar / poll |
| `IN_PROGRESS` | Pagamento em processamento | Aguardar / poll |
| `PAID` | Pagamento confirmado | **Liberar a assinatura anual** a partir de `paid_at` |
| `DECLINED` | Pagamento recusado | Oferecer novo checkout / tentar de novo |
| `EXPIRED` | Link expirou sem pagamento | Oferecer novo checkout |
| `CANCELLED` | Cancelado | Não liberar |
| `ERROR` | Erro no provedor | Não liberar; logar para investigação |

## Passo 5 — Apresentar o resultado ao comprador

Com a resposta confirmada do passo 4, monte a página de retorno do seu site de acordo com
`capture_method`/`installments`:

- **Pix** (`capture_method: "pix"`, `installments: 1`): "Pagamento confirmado via Pix! Sua
  assinatura anual está ativa." + link para `receipt_url`.
- **Cartão em N parcelas** (`capture_method: "credit_card"`, `installments: 3`): "Pagamento
  confirmado — 3x no cartão de crédito. Sua assinatura anual está ativa até
  `paid_at + 1 ano`" (calculado no seu backend).
- **Ainda não confirmado**: "Estamos confirmando seu pagamento, isso pode levar alguns segundos" +
  retry/polling no front-end, ou orientação para checar e-mail depois.

## Operações sem suporte (não tente)

A InfinitePay não oferece, e portanto o banking retorna erro explícito para `baas=infinitepay` em:
- **Cobrança direta/recorrente** (não existe `Authorize` funcional para este provedor).
- **Cancelamento de link via API** (`PUT /v1/checkouts/{id}/cancel` retorna erro para
  `baas=infinitepay` — não trate isso como um bug, é o comportamento esperado).

## Checklist rápido de implementação

### Plano Mensal
- [ ] `GET /v1/subscriptions/infinitepay/plans/monthly` para obter o link do plano.
- [ ] Redirecionar o comprador para `url` retornada.
- [ ] Nenhuma outra integração necessária — recorrência gerenciada pela InfinitePay.

### Plano Anual
- [ ] Enviar `X-API-Key` em todas as chamadas.
- [ ] `POST /v1/subscriptions` com `baas: "infinitepay"`, `plan`, `redirect_url` — **sem valor**.
- [ ] Redirecionar o navegador para `checkout_url` da resposta.
- [ ] Na página de `redirect_url`, extrair `slug` da query string só para saber o que consultar —
      nunca para decidir se o pagamento foi confirmado.
- [ ] Chamar `GET /v1/checkouts/{checkout_id}?baas=infinitepay` e agir só com base no `status`.
- [ ] Tratar `PAID` liberando a assinatura anual; tratar `DECLINED`/`EXPIRED` criando novo checkout.
- [ ] Não implementar cancelamento/cobrança recorrente direta via API para `infinitepay`.
