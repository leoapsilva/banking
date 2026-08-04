# Modelo alternativo: uma única InfiniteTag compartilhada entre 1000 tenants

Cenário levantado: em vez de cada tenant ter sua própria InfiniteTag (ver
[MULTITENANCY_ISOLAMENTO_INFINITEPAY.md](MULTITENANCY_ISOLAMENTO_INFINITEPAY.md)), todos os 1000
clientes do app usam a InfiniteTag do operador do banking. O operador repassaria os valores
(descontando as taxas de parcelamento cobradas pela InfinitePay) e cobraria uma taxa fixa por
transação, emitindo nota fiscal de serviço pela gestão dos pagamentos.

**Status**: ideia em avaliação, decisão não tomada. Este documento registra a análise técnica e os
pontos legais levantados, não uma decisão de arquitetura.

## 1. Como diferenciar cada transação para repasse posterior

Com `handle` fixo (a conta do operador), a única variável que sobra para rastrear "de qual tenant é
essa transação" é o que o banking coloca no `order_nsu` e o que é persistido **antes** de chamar a
InfinitePay — a API deles não tem campo de sub-conta/sub-merchant.

### Atribuição na criação
- O `service` resolve `tenant_id` a partir da autenticação do chamador. Antes de chamar
  `POST /links`, grava-se na tabela `checkouts`: `tenant_id`, `order_nsu` (gerado pelo banking, ex.
  `T{tenant_id}-{seq}`), `slug` (preenchido após a resposta), `amount_cents`, e os termos de taxa
  aplicáveis.
- `order_nsu` enviado à InfinitePay carrega esse vínculo de forma opaca para eles, mas decifrável
  pelo banking.

### Atribuição no recebimento (webhook)
- O webhook chega com `invoice_slug`/`order_nsu`/`transaction_nsu`, sem nenhuma referência a tenant.
  A linha é buscada por `slug`/`order_nsu`, e o `tenant_id` vem do registro **próprio**, nunca do
  payload — mesma técnica de correlação descrita na seção 3b do
  [RELEASE_0.1.0.md](RELEASE_0.1.0.md), agora servindo também para reconciliação financeira.

### Limite técnico importante
A documentação lida não expõe a *taxa efetivamente cobrada pela InfinitePay* por transação (só
`amount` vs `paid_amount`, que parece refletir o valor pago pelo comprador com juros de
parcelamento — não a taxa retida do operador). Para saber o líquido exato por transação, seria
necessário:
- Manter uma tabela própria de **taxas por modalidade/parcela** (capture_method + installments → %
  de taxa) para estimar o líquido no momento do recebimento, **e**
- Conciliar periodicamente contra o extrato real da conta InfinitePay (acesso direto, fora desta
  API) para confirmar o valor líquido exato — sem isso, o repasse ao tenant seria uma estimativa, não
  um valor garantido.

### Módulo novo necessário
O módulo `checkout` atual só rastreia status (`PAID`/`CREATED`/etc.), não dinheiro devido. Seria
necessário um **ledger/settlement** por tenant: valor bruto, taxa InfinitePay (estimada/conciliada),
taxa fixa do operador, valor líquido a repassar, status do repasse (pendente/pago).

Esse módulo poderia reusar o `internal/pix` já existente no projeto para a perna de saída (repasse
via PIX para a conta do tenant) — sinergia natural, já que o banking já tem capacidade de emitir PIX.

## 2. Implicações legais (pontos para advogado/contador — não é parecer jurídico)

Esse desenho — receber em nome próprio e redistribuir a terceiros, cobrando uma taxa — coloca o
operador na posição de **subadquirente/facilitador de pagamentos** por cima da InfinitePay, mesmo
sem ser uma instituição de pagamento registrada.

### a) Regulação do Banco Central (Lei 12.865/2013, Resolução BCB nº 80)
Receber e movimentar fundos de terceiros como atividade comercial recorrente pode caracterizar
"arranjo de pagamento"/"instituição de pagamento", dependendo de volume e estrutura — trazendo
obrigação de registro/autorização no BACEN, segregação de contas (valores de terceiros não deveriam
se misturar ao caixa operacional), e compliance de PLD/KYC (Lei 9.613/1998, reporte ao COAF).
Recomenda-se perguntar explicitamente a advogado especializado em meios de pagamento se o volume
projetado (1000 tenants) ultrapassa o limiar que exigiria registro.

### b) Repasse da taxa da InfinitePay ao tenant
O contrato com a InfinitePay é do operador, não do tenant. Repassar esse custo ao tenant é
comercialmente normal (modelo de qualquer marketplace/agregador), mas precisa estar **explícito no
contrato com o tenant** (termos de uso/SLA: "taxa de repasse da adquirente + taxa de serviço"), senão
pode ser interpretado como margem oculta.

### c) Nota fiscal de serviço — base de cálculo
A NFS-e por "gestão de pagamentos" deveria refletir **apenas a receita do operador** (taxa fixa +
margem sobre o repasse), não o valor bruto transacionado (GMV) — o valor bruto é receita do tenant
passando pela custódia do operador, não receita própria. Faturar o bruto infla artificialmente a
base de ISS/PIS-COFINS e cria exposição tributária real. Mesmo princípio contábil usado por
marketplaces (reconhecimento de receita só sobre o *take rate*, não sobre o GMV) — confirmar com
contador antes de emitir a primeira nota.

### d) Custódia de dinheiro de terceiros
Entre a InfinitePay creditar a conta do operador e o repasse ao tenant, o operador fica com dinheiro
de terceiros em nome próprio. Isso levanta:
- Contabilmente, esse valor não deveria entrar como receita/ativo do operador — é um passivo
  ("valores a repassar a terceiros").
- Em caso de problema financeiro do operador, esse dinheiro pode ficar emaranhado nos passivos da
  empresa, expondo tenants a risco de não receber — contrato deveria endereçar isso com cláusulas de
  prazo de repasse e possivelmente linguagem de natureza fiduciária.

### e) LGPD
O operador passaria a processar dados pessoais (nome/e-mail/telefone/endereço) de clientes finais de
1000 tenants diferentes — revisar se os contratos com os tenants endereçam o papel do operador como
operador/controlador desses dados, especialmente considerando a persistência de
`payer_email`/`payer_phone`/`payer_address` planejada em [RELEASE_0.1.0.md](RELEASE_0.1.0.md).

## Recomendação

Antes de avançar com esse modelo, validar com advogado especializado em meios de pagamento/fintech e
com contador: (i) se o volume projetado exige registro como instituição de pagamento, e (ii) a base
de cálculo correta da NFS-e. A parte técnica (ledger, conciliação, repasse via PIX) pode ser desenhada
em paralelo, mas a decisão de modelo depende dessas respostas.
