# Release 0.1.0 — Checkout InfinitePay

## Contexto

O repositório já implementa checkout (e outras features: pix, boleto, payment-scheduling) sobre uma
arquitetura agnóstica de provedor: `internal/checkout/{domain,port.go,handler,service,repository}` +
um adapter por BaaS em `internal/provider/<baas>/`, ligados via `providerregistry` (chave
`feature:baas`). Hoje só existe o provedor C6 (`internal/provider/c6/`).

A documentação salva em `docs/infinitepay/docs/Conta Web InfinitePay.html` descreve a API de
**Checkout/Link de Pagamento da InfinitePay**:

- `POST https://api.checkout.infinitepay.io/links` — cria um link de pagamento hospedado.
  - Obrigatório: `handle` (InfiniteTag sem `$`), `items` (≥1, cada `{quantity, price (centavos), description}`).
  - Opcional: `order_nsu`, `redirect_url`, `webhook_url`, `customer {name, email, phone_number}`,
    `address {cep, street, neighborhood, number, complement}`.
  - Resposta não documentada explicitamente na página, mas os parâmetros devolvidos via
    `redirect_url`/webhook (`slug`, `transaction_nsu`, `capture_method`, `receipt_url`) indicam que a
    criação devolve ao menos uma URL de checkout e um `slug`.
- `POST https://api.checkout.infinitepay.io/payment_check` — consulta status, exige
  `handle + order_nsu + transaction_nsu + slug`. `transaction_nsu` só é conhecido depois que o
  pagamento ocorre (via webhook ou retorno do redirect) — **não está disponível no momento da criação**.
- Webhook: InfinitePay faz POST para a `webhook_url` informada na criação do link quando o pagamento é
  aprovado, payload `{invoice_slug, amount, paid_amount, installments, capture_method, transaction_nsu,
  order_nsu, receipt_url, items}`. Espera resposta `200 OK` (rápida); `400` causa retentativa.
- **Não documentado**: API key/Bearer/autenticação (a única credencial é o `handle`, que é público),
  endpoint de cancelamento/expiração de link, assinatura HMAC do webhook, ambiente sandbox, rate
  limit/idempotência.

Isso é estruturalmente diferente do C6: é um fluxo de link hospedado (sem carga direta de cartão
tokenizado, sem cancelamento via API, sem GET por id). Decisões de adaptação confirmadas:

- `Authorize()` e `Cancel()` no adapter InfinitePay retornam um erro tipado "não suportado" — a
  InfinitePay não oferece cobrança direta nem cancelamento de link via API.
- `Get(providerCheckoutID)` é **DB-first / webhook-driven**: nunca chama a InfinitePay diretamente.
  Lê o status já persistido no `checkout` repository (atualizado pelo handler de webhook inbound).
  `ProviderCheckoutID` = o `slug` devolvido por `/links`.

## Como o cliente (consumidor da nossa API) vai usar o checkout InfinitePay

Aqui "cliente" é quem integra com a **nossa** API (`/v1/checkouts`), não o comprador final. O fluxo é
o mesmo desenho já usado para C6, só troca `"baas"`:

1. **Criar o checkout** — `POST /v1/checkouts` com `"baas": "infinitepay"`:
   ```json
   {
     "baas": "infinitepay",
     "external_reference_id": "PEDIDO-0001",
     "amount_cents": 150000,
     "currency": "BRL",
     "description": "Pedido #0001",
     "redirect_url": "https://seusite.com/pagamento-concluido",
     "payer": {
       "name": "Maria Souza",
       "email": "maria@email.com",
       "phone_number": "11999998888",
       "address": {
         "zip_code": "12345678",
         "street": "Rua das Flores",
         "neighborhood": "Centro",
         "number": "123",
         "complement": "Apto 45"
       }
     }
   }
   ```
   `card` continua não se aplicando (a InfinitePay não aceita cartão tokenizado via API — quem
   escolhe a forma de pagamento é o comprador, na página hospedada). Mas `payer`/`address` **são
   enviados e usados**: a InfinitePay aceita `customer{name,email,phone_number}` e
   `address{cep,street,neighborhood,number,complement}` no `POST /links` para pré-preencher a página
   de pagamento. O `mapper` do adapter InfinitePay traduz `domain.Payer`/`domain.Address` para esse
   formato. `TaxID` (CPF) não tem campo correspondente documentado na InfinitePay — é ignorado por
   esse adapter (mas continua existindo no domínio para o C6, que o exige).
   **Persistência**: hoje a tabela `checkouts` só guarda `payer_tax_id`/`payer_name` (ver
   `internal/checkout/repository/repository.go`). Para reter e-mail/telefone/endereço também ("eu
   fico com esta informação no meu DB"), a migration adiciona colunas `payer_email`, `payer_phone`,
   `payer_address jsonb` em `checkouts`, populadas pelo `service` ao chamar `Repository.Create` a
   partir do `domain.Payer`/`domain.Address` do request — isso é genérico (qualquer baas que enviar
   esses dados se beneficia), não específico da InfinitePay.
   Como `domain.Address` hoje não tem campo `Neighborhood` (só `Street, Number, Complement, City,
   State, ZipCode`), adiciona-se um campo opcional `Neighborhood string` ao `domain.Address` — C6
   ignora, InfinitePay usa.
   Internamente o `service` chama o `CheckoutAdapter.Create`, que injeta `handle` e `webhook_url`
   fixos (config do nosso lado, não vêm do cliente) e faz `POST /links` na InfinitePay.
   **Resposta para o cliente**: `{id, url, status: "CREATED"}` — `id` é o `slug` devolvido pela
   InfinitePay (usado como `ProviderCheckoutID`), `url` é o link de pagamento hospedado.

2. **Redirecionar o comprador** — o cliente da nossa API pega `url` da resposta acima e redireciona o
   comprador final para lá (página da InfinitePay, fora do nosso domínio). O comprador escolhe pix ou
   cartão diretamente na página da InfinitePay.

3. **Saber quando foi pago** — diferente do C6, o cliente **não deve fazer polling em `GET
   /v1/checkouts/{id}`** esperando que isso dispare uma chamada à InfinitePay (não existe esse
   endpoint do lado deles). Duas formas de saber do pagamento, ambas internas/automáticas:
   - **Webhook (recomendado)**: a InfinitePay nos notifica em `POST
     /webhooks/infinitepay/{urlToken}` quando o comprador paga; nosso `webhook` service atualiza o
     registro do checkout para `PAID` no banco. O cliente da nossa API não recebe esse webhook
     diretamente — ele continua consultando `GET /v1/checkouts/{id}?baas=infinitepay`, e essa rota
     simplesmente lê o status já atualizado no nosso banco (não chama a InfinitePay de novo).
   - **Redirect de volta**: se o cliente configurou `redirect_url`, o comprador é trazido de volta com
     query params (`order_nsu`, `slug`, `transaction_nsu`, `capture_method`, `receipt_url`) — útil para
     UX imediata ("Pagamento confirmado!"), mas não deve ser a única fonte de verdade (comprador pode
     fechar a aba antes do redirect).
   - Enquanto nenhum webhook chegou, `GET /v1/checkouts/{id}` simplesmente devolve o último status
     conhecido (`CREATED` ou `IN_PROGRESS`), nunca um erro — é uma leitura local, não uma chamada
     externa que pode falhar.

4. **Operações sem suporte** — se o cliente tentar `Authorize` (cobrança direta de cartão salvo) ou
   `PUT /v1/checkouts/{id}/cancel` com `baas=infinitepay`, a resposta é um erro claro (ex.: HTTP 400/501
   com `{"error": "operation not supported by infinitepay"}`), em vez de um 500 genérico ou uma
   tentativa silenciosa que não faz nada. Isso é decisão deliberada: a InfinitePay não oferece essas
   operações via API.

## Mudanças

### 1. Domínio (`internal/checkout/domain/checkout.go`)
- Adicionar `BaaSInfinitePay BaaS = "infinitepay"` à lista de constantes (junto de `BaaSC6`).
- Adicionar erro sentinela compartilhado (ex.: em `internal/checkout/port.go` ou `domain`):
  `ErrNotSupported = errors.New("operation not supported by this provider")`, usado pelo adapter
  InfinitePay em `Authorize`/`Cancel` e tratado pelo `service` como uma resposta de erro
  (ex.: 501/400) em vez de propagar como falha genérica.
- Adicionar `Neighborhood string` (opcional) à struct `Address` — usado pela InfinitePay
  (`address.neighborhood`), ignorado pelo C6 hoje.

### 1b. Persistência de payer/address (`internal/checkout/repository/repository.go` + migration)
- Nova migration: `ALTER TABLE checkouts ADD COLUMN payer_email text, ADD COLUMN payer_phone text,
  ADD COLUMN payer_address jsonb;`.
- `Checkout` struct ganha `PayerEmail, PayerPhone string` e `PayerAddress *domain.Address`
  (serializado como jsonb).
- `Repository.Create`/`GetByProviderID`/`GetByID` passam a gravar/ler essas colunas. Isso é uma
  melhoria genérica do módulo `checkout` (não exclusiva da InfinitePay): hoje só `payer_tax_id` e
  `payer_name` são persistidos; e-mail/telefone/endereço chegam no request mas são descartados.
- `service/service.go`: ao montar `repository.Checkout` para `Create`, passar
  `req.Payer.Email/PhoneNumber/Address` (com nil-check, já que `Payer` é `*Payer`).

### 2. Novo provider (`internal/provider/infinitepay/`)
Seguindo o padrão de `internal/provider/c6/`, mas mais simples (sem mTLS/OAuth):

- `client/client.go` — wrapper HTTP simples: `New(httpClient *http.Client, baseURL string)`,
  método `Do(ctx, method, path, reqBody, respBody)`. Sem token manager (não há autenticação).
- `dto/checkout.go` — shapes exatos da API InfinitePay:
  - `CreateLinkRequest{Handle, Items []Item, OrderNSU, RedirectURL, WebhookURL, Customer, Address}`
  - `Item{Quantity int, Price int64 (centavos), Description string}`
  - `CreateLinkResponse` — campos conhecidos (`slug`/`url` na ausência de doc explícita; usar o
    nome de campo mais provável e validar contra o ambiente real/sandbox da conta antes de
    finalizar — ver seção Verificação)
  - `PaymentCheckRequest{Handle, OrderNSU, TransactionNSU, Slug}`
  - `PaymentCheckResponse{Success, Paid bool, Amount, PaidAmount int64, Installments int, CaptureMethod string}`
  - `WebhookPayload{InvoiceSlug, Amount, PaidAmount int64, Installments int, CaptureMethod,
    TransactionNSU, OrderNSU, ReceiptURL string, Items []Item}`
- `mapper/checkout.go` — funções puras, mesmo estilo de `provider/c6/mapper/checkout.go`:
  - `ToCreateLinkRequest(handle string, req domain.CreateCheckoutRequest) dto.CreateLinkRequest`
    (mapeia `Description`+`Amount` → um único item, já que o domínio atual não tem lista de itens;
    `Amount.AmountCents` → `Item.Price`; mapeia `req.Payer` → `dto.Customer{Name, Email,
    PhoneNumber}` e `req.Payer.Address` → `dto.Address{Cep: ZipCode, Street, Neighborhood, Number,
    Complement}` quando `Payer != nil`, ambos opcionais na chamada à InfinitePay)
  - `FromCreateLinkResponse(resp dto.CreateLinkResponse) domain.CheckoutResult` (status sempre
    `StatusCreated`)
  - `FromWebhookPayload(p dto.WebhookPayload)` — usado pelo InboundParser, não aqui diretamente
- `checkout_adapter.go` — implementa `checkout.Provider`:
  - `NewCheckoutAdapter(c *client.Client, handle, webhookURL string)`
  - `Create()`: monta `CreateLinkRequest` (injeta `Handle` e `WebhookURL` fixos de config),
    `POST /links`, mapeia resposta.
  - `Authorize()`: retorna `domain.ErrNotSupported` imediatamente, sem chamada HTTP.
  - `Get(ctx, providerCheckoutID)`: **não chama a API** — ver decisão acima; na prática este método
    do adapter deve ficar inativo/retornar erro também, porque a leitura real acontece no `service`
    via repository, não no adapter. Avaliar se `Get` deveria nem ser chamado pelo `service` quando
    `BaaS == infinitepay` (checar `service/service.go` atual para ver onde o `Get` é invocado e
    ajustar para ler do repositório diretamente antes de cair no adapter, ou simplesmente fazer o
    adapter `Get` retornar `ErrNotSupported` e o `service` tratar isso lendo do repo como fallback).
  - `Cancel()`: retorna `domain.ErrNotSupported`.
- (Opcional, fase posterior) `payment_check` como método auxiliar não exposto pela interface
  `Provider` — usado apenas internamente pelo webhook adapter se for necessário confirmar valores,
  mas não é estritamente necessário já que o próprio webhook já entrega `paid`/`amount`.

### 3. Webhook inbound (`internal/provider/infinitepay/webhook_adapter.go`)
Mesmo padrão de `provider/c6/webhook_adapter.go`, implementando `webhook.InboundParser`:
- `ParseInbound(ctx, rawBody)`: decodifica `dto.WebhookPayload`, monta
  `webhookdomain.InboundEvent{BaaS: "infinitepay", Service: ServiceCheckout, Status: <derivar de
  Paid/CaptureMethod>, ProviderCheckoutID: payload.InvoiceSlug, RawPayload: rawBody, ...}`.
- **Não implementa `RegistrarProvider`** de forma significativa: a InfinitePay não tem um endpoint de
  inscrição de webhook por serviço como o C6 — a `webhook_url` é enviada em cada `POST /links`. Então
  `Register`/`Delete` podem simplesmente não ser chamados para este BaaS (não registrar no
  `providerregistry` sob a chave `webhook:infinitepay`, ou registrar um adapter que retorna
  `ErrNotSupported`).
- Status mapping: documentação não expõe um campo de status textual no webhook (só `paid: true`
  implícito pelo fato de o webhook disparar só em aprovação) — mapear sempre para `StatusPaid` ao
  receber o webhook, já que a InfinitePay só notifica em caso de pagamento aprovado.

### 3b. Anti-forjamento do webhook InfinitePay
A InfinitePay não documenta HMAC nem qualquer identificador de conta no corpo do webhook (diferente
do C6, que manda `client_id`/`partner_id` validáveis via `ExpectedOrigin`). Sem isso, qualquer um que
descubra a URL do nosso webhook poderia forjar um payload de "pago". Mitigação em duas camadas:

1. **`urlToken` na rota** (já no item 4) — obscuridade básica, igual ao padrão do C6.
2. **Correlação contra o registro que nós criamos** — antes de marcar como `PAID`, o
   `checkoutSink.HandleCheckoutEvent` (ou um passo antes, no próprio `InboundParser`/`service`) deve
   buscar o checkout local por `ProviderCheckoutID` (= `invoice_slug`) e confirmar que
   `order_nsu`/`amount` do payload batem com o que está persistido e que o checkout ainda não está em
   status terminal. Se não bater ou não existir, o evento é rejeitado/logado como suspeito em vez de
   aplicado — assim um atacante precisaria adivinhar o `urlToken` **e** um `slug`/`order_nsu`
   válidos nossos, não bastando só um dos dois.
3. **Dedupe/idempotência adaptada**: a tabela `webhook_events` hoje dedupe por
   `(baas, external_id, status, event_date_time)`, chave que existe no envelope do C6 mas não no da
   InfinitePay (sem `external_id`/timestamp). Para InfinitePay, usar `transaction_nsu + invoice_slug`
   como chave de dedupe equivalente.
4. **Rate limiting / WAF na borda** (proxy/CDN) continua sendo a defesa real contra flood/DoS no
   endpoint — fora do código da aplicação, mas vale registrar como pré-requisito de infraestrutura
   antes de expor a rota em produção (mesma exposição que o webhook do C6 já tem hoje).

Sem mTLS/HMAC reais, isso fica em "obscuridade + correlação", não criptograficamente garantido — é
limitação da própria API da InfinitePay, não escolha nossa.

### 3c. (Fora do escopo desta release) Relay de notificação ao cliente da nossa API
Hoje o `webhook` module só processa o evento inbound da InfinitePay/C6 e atualiza nosso banco via
`checkoutSink.HandleCheckoutEvent` — não existe nenhum mecanismo de reenviar a notificação para uma
URL do cliente da nossa API. Ou seja, o cliente **não pode** usar sua própria rota como `webhook_url`
da InfinitePay (ver seção "Como o cliente vai usar"); ele só sabe do pagamento consultando
`GET /v1/checkouts/{id}`.

Se no futuro quisermos notificação em tempo real para o cliente, seria uma feature nova e separada:
- Campo opcional `notify_url` em `CreateCheckoutRequest`, persistido em `checkouts` (nova coluna).
- Depois que `HandleCheckoutEvent` atualizar o status no banco, disparar um `POST` assíncrono pra
  `notify_url` com `{checkout_id, external_reference_id, status}` (com retry/backoff básico).
- Não está incluído nas mudanças desta release — registrado aqui só para não perder a decisão de
  que é um gap consciente, não um descuido.

### 4. Roteamento do webhook inbound (`internal/webhook/handler/handler.go`)
- Adicionar rota `POST /webhooks/infinitepay/{urlToken}` análoga a `receiveC6`, reaproveitando
  `pathSecret` (mesma técnica de obscuridade por URL, já que a InfinitePay não documenta assinatura
  HMAC) — pode reusar o mesmo secret de config ou um novo `INFINITEPAY_WEBHOOK_URL_TOKEN`.
- `service.ProcessInbound(ctx, "infinitepay", body)` — reaproveita o método existente, que já
  despacha por BaaS via os `InboundParser`s registrados.

### 5. Wiring (`cmd/api/main.go`)
- Ler novas envs: `INFINITEPAY_BASE_URL` (default `https://api.checkout.infinitepay.io`),
  `INFINITEPAY_HANDLE`, `INFINITEPAY_WEBHOOK_URL` (URL pública do nosso endpoint de webhook),
  `INFINITEPAY_WEBHOOK_URL_TOKEN`.
- Criar `infinitepay/client.Client` (http.Client padrão, sem mTLS), `CheckoutAdapter`,
  `WebhookAdapter`.
- `providerregistry.Register(registry, "checkout", "infinitepay", infinitePayCheckoutAdapter)`.
- Registrar o `InboundParser` da InfinitePay junto do webhook `service` (checar como C6 é registrado
  hoje no `webhook/service.Service` — provavelmente um map por BaaS dentro do próprio `service`, não
  via `providerregistry`, já que `webhook.Service` referencia parsers diretamente).

### 6. Config (`.env.example`, `docker/.env.example`)
Adicionar as novas variáveis acima com comentários curtos, no mesmo estilo dos blocos `C6_*`.

### 7. Testes Bruno (`tests/bruno/checkouts/`)
Adicionar exemplos análogos aos existentes:
- Criar checkout: `POST /v1/checkouts` com `"baas": "infinitepay"`, payload mínimo (sem `card`, já
  que não se aplica — `description`+`amount_cents` bastam).
- Documentar no `tests/bruno/README.md` que `authorize`/`cancel` retornam erro para `infinitepay`.

### 8. Documentação (`docs/infinitepay/`)
Adicionar um `docs/infinitepay/README.md` ou `swagger/checkout-infinitepay.yaml` resumindo a API
real (extraída da página HTML), no mesmo espírito de `docs/c6/swagger/`, já que o HTML original não é
prático para consulta rápida.

## Arquivos-chave a criar/editar
- `internal/checkout/domain/checkout.go` (editar: nova constante BaaS + erro sentinela + `Neighborhood`)
- `internal/checkout/repository/repository.go` (editar: novas colunas payer_email/payer_phone/payer_address)
- `internal/checkout/service/service.go` (editar: popular novos campos de payer + estratégia de `Get`)
- `internal/provider/infinitepay/client/client.go` (novo)
- `internal/provider/infinitepay/dto/checkout.go` (novo)
- `internal/provider/infinitepay/mapper/checkout.go` (novo)
- `internal/provider/infinitepay/checkout_adapter.go` (novo)
- `internal/provider/infinitepay/webhook_adapter.go` (novo)
- `internal/webhook/handler/handler.go` (editar: nova rota)
- `cmd/api/main.go` (editar: wiring)
- `.env.example`, `docker/.env.example` (editar)
- `tests/bruno/checkouts/` (novo .bru)

## Verificação
1. `go build ./...` e `go vet ./...` para garantir que o novo pacote compila e a interface
   `checkout.Provider` é satisfeita.
2. Testes unitários do mapper (`mapper/checkout_test.go`), espelhando
   `provider/c6/mapper/checkout_test.go`.
3. Rodar a API localmente (`cmd/api`), usar o Bruno (`tests/bruno/run.sh` ou requests manuais) contra
   um `handle` de sandbox/teste real da InfinitePay para confirmar o shape real da resposta de
   `POST /links` (campo de URL/slug não está 100% documentado na página salva — validar contra a API
   viva antes de fechar o `dto.CreateLinkResponse`).
4. Simular um webhook inbound (`curl -X POST .../webhooks/infinitepay/<secret>` com o payload de
   exemplo da doc) e confirmar que o `checkout` correspondente passa para `PAID` no banco.
5. Confirmar que `Authorize`/`Cancel` para `baas=infinitepay` retornam um erro HTTP claro (não 500
   genérico) via `tests/bruno/checkouts/`.
