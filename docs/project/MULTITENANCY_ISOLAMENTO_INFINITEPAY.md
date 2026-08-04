# Multitenancy e isolamento — checkout InfinitePay por tenant

Questão levantada: se o cliente do banking é um app multitenant de administração de serviços, e cada
tenant configura sua própria InfiniteTag, como garantir que um tenant nunca recebe (ou tem acesso a)
pagamento de outro?

## Por que o plano atual não suporta isso diretamente

O [RELEASE_0.1.0.md](RELEASE_0.1.0.md) foi desenhado **single-tenant**: um `handle` e um
`webhook_url` fixos via env var, configurados uma vez no deploy. Para múltiplos tenants, cada um com
seu próprio InfiniteTag, isso precisa virar dado por tenant, não config global.

## Pontos críticos de isolamento

### a) Quem define o `handle` enviado ao `/links` não pode vir do request do tenant
Se o `handle` viesse direto no payload de criação do checkout, um tenant poderia — por erro ou
má-fé — mandar o InfiniteTag de outro tenant, e o dinheiro cairia na conta errada. O `handle`
precisa ser **resolvido no servidor** a partir da identidade autenticada do tenant (API key →
tenant_id → InfiniteTag configurado em uma tabela `tenants`), nunca aceito como campo do request.

Isso muda o `CheckoutAdapter` planejado: hoje o construtor injeta `handle` fixo
(`NewCheckoutAdapter(c, handle, webhookURL)`); para multitenant, `handle` passa a ser parâmetro por
chamada de `Create()`, vindo do lookup do tenant.

### b) Atribuição no webhook é o ponto mais delicado
O payload do webhook da InfinitePay **não traz `handle` nem qualquer identificador de conta** — só
`invoice_slug, order_nsu, transaction_nsu, amount, ...`. Não há como saber de qual tenant é o
pagamento só olhando o corpo do webhook.

Solução: ao criar o checkout, gravamos `tenant_id` junto com `provider_checkout_id` (slug) na nossa
tabela. Quando o webhook chega com `invoice_slug`, buscamos a linha **nossa** por esse slug e o
`tenant_id` vem de lá — nunca do payload da InfinitePay. É a mesma correlação anti-forjamento da
seção 3b do plano de release, agora servindo também como mecanismo de isolamento.

### c) Chaves de busca devem incluir tenant
`repository.GetByProviderID` hoje é `(baas, provider_checkout_id)`. Para multitenant, mais seguro
tratar como `(baas, tenant_id, provider_checkout_id)` — mesmo sem evidência de que a InfinitePay
reusa slugs entre contas diferentes, não vale confiar em unicidade global não documentada.

### d) Autorização por tenant nas leituras
Hoje não existe conceito de tenant no domínio/repositório (`Checkout` não tem `TenantID`). Sem isso,
qualquer chamador autenticado poderia consultar o checkout de outro tenant só sabendo o `id`.
Necessário: middleware de auth resolvendo `tenant_id` do chamador, e todo `Get`/`UpdateStatus`
filtrando por esse `tenant_id`.

## Resumo

É viável, mas exige introduzir `tenant_id` como conceito de primeira classe:
- Tabela de tenants com handle configurado.
- Coluna `tenant_id` em `checkouts`.
- Resolução de `handle` no servidor (nunca aceito do request).
- Atribuição de webhook via correlação no nosso banco, não via payload da InfinitePay.
- Autorização por tenant em toda leitura/escrita.

Isso é mudança de escopo em relação ao [RELEASE_0.1.0.md](RELEASE_0.1.0.md) atual, que assume um
handle único. Ver também [TRADEOFFS_DEPLOYMENT_MULTITENANT.md](TRADEOFFS_DEPLOYMENT_MULTITENANT.md)
para as alternativas de design de deployment que reforçam (ou não) esse isolamento, e
[MODELO_INFINITETAG_COMPARTILHADA.md](MODELO_INFINITETAG_COMPARTILHADA.md) para o cenário alternativo
de uma única InfiniteTag compartilhada entre todos os tenants.
