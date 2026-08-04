# Webhook vs. Redirect URL — InfinitePay Checkout

Esclarecimento de conceitos levantado durante o planejamento do checkout InfinitePay
(ver [RELEASE_0.1.0.md](RELEASE_0.1.0.md)).

## Webhook (`webhook_url`)

É uma URL **do banking** (`/webhooks/infinitepay/{urlToken}`) que a InfinitePay chama
(server-to-server) quando o pagamento é aprovado. É o mecanismo que evita polling: o banking fica
sabendo do pagamento de forma assíncrona e atualiza o status no banco sozinho.

- É fixa por configuração (env var), não por checkout — ver decisão de arquitetura em
  [RELEASE_0.1.0.md](RELEASE_0.1.0.md), seção "Como o cliente vai usar".
- A rota do site do cliente da nossa API **não** deve ser usada como `webhook_url` direto na
  InfinitePay, porque isso faria a notificação pular o banking — nosso `GET /v1/checkouts/{id}`
  (estratégia DB-first/webhook-driven) nunca saberia que o pagamento ocorreu.

## Redirect (`redirect_url`)

É uma URL **do chamador da API** (site/app do tenant), mas não é um callback de servidor — é para
onde o **navegador do comprador** é levado depois de pagar, com parâmetros na query string
(`order_nsu`, `slug`, `transaction_nsu`, `capture_method`, `receipt_url`).

- Controlada pelo chamador, pode variar por checkout (diferente do `webhook_url`, que é fixo).
- É um redirecionamento de **navegador**, não uma chamada de API confiável — o comprador pode fechar
  a aba antes de chegar lá. Não deve ser a única fonte de verdade sobre o pagamento.
- O webhook continua sendo a fonte de verdade; o redirect é só UX ("Pagamento confirmado!").

## Resumo

| | Quem chama | Direção | Confiabilidade | Variação |
|---|---|---|---|---|
| `webhook_url` | InfinitePay → banking | server-to-server | Alta (fonte de verdade) | Fixa, nossa |
| `redirect_url` | Navegador do comprador | browser redirect | Baixa (best-effort) | Por checkout, do tenant |
