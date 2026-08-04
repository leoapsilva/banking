# Trade-offs de design de deployment para isolamento multitenant

Análise solicitada: comparar "pod/instância dedicada por tenant" contra outras alternativas de
system design/deployment para isolar tenants no banking, antes de decidir se o sistema vai virar
multitenant. Ver questão original em
[MULTITENANCY_ISOLAMENTO_INFINITEPAY.md](MULTITENANCY_ISOLAMENTO_INFINITEPAY.md).

Contexto de risco: isolar tenant aqui não é só "não vazar dado de leitura" — é "não redirecionar
pagamento pra conta errada". O pior caso tem consequência financeira direta, não só de privacidade.

## Dimensões avaliadas

- **Garantia de isolamento**: lógica (código/query) vs. física (processo/rede/banco separados)
- **Blast radius de um bug**: um erro de filtro de `tenant_id` afeta 1 tenant ou todos?
- **Custo operacional**: quantos artefatos pra deployar/monitorar/atualizar conforme tenants crescem
- **Eficiência de recursos**: ociosidade de CPU/memória/conexões de banco por tenant inativo
- **Velocidade de onboarding**: criar um tenant novo é "INSERT numa tabela" ou "pipeline de infra"?

## Design A — Pod/instância dedicada por tenant

Cada tenant tem seu próprio deployment (`cmd/api` isolado), possivelmente seu próprio banco/schema,
suas próprias envs (`INFINITEPAY_HANDLE`, secrets).

- **Isolamento**: o mais forte possível sem ser isolamento de rede física. Não há `tenant_id` pra
  esquecer de filtrar, porque não há outro tenant no processo.
- **Blast radius**: mínimo. Pod de um tenant cai, vaza memória, é comprometido → só aquele tenant é
  afetado.
- **Custo operacional**: cresce linearmente (e mal) com o número de tenants — N pods, N
  configmaps/secrets, N pipelines de deploy, N migrations de banco. Trivial para 5 tenants;
  operação de plataforma em si para 200+.
- **Eficiência de recursos**: baixa — cada pod tem overhead fixo (connection pool, health checks,
  etc.). Tenants de baixo volume pagam o mesmo custo-base de um tenant grande.
- **Onboarding**: requer pipeline de infra (criar pod, secret, possivelmente banco), não é mais
  "criar uma linha numa tabela".

## Design B — Shared app + isolamento lógico por `tenant_id`

Um único deployment, uma base de dados, `tenant_id` em toda tabela relevante, resolvido a partir da
autenticação do chamador.

- **Isolamento**: depende de disciplina de código — toda query precisa filtrar por `tenant_id`.
  Reforçável com **Row-Level Security do Postgres** (policy `tenant_id = current_setting(...)`),
  que transforma "lembrar de filtrar" em "o banco recusa por padrão" — reduz bastante o risco de um
  `WHERE` esquecido, mas ainda é isolamento lógico, não físico.
- **Blast radius**: um bug de RLS/filtro mal configurado pode expor múltiplos tenants de uma vez —
  esse é o risco real desse modelo.
- **Custo operacional**: mínimo — é o desenho atual só com uma tabela de tenants a mais. Onboarding
  = inserir configuração (handle, credenciais).
- **Eficiência de recursos**: ótima — pool de conexões compartilhado, um binário, escala com
  tráfego total, não com número de tenants.
- Modelo padrão de mercado para SaaS B2B (ex.: Stripe Connect opera assim).

## Design C — Shared app + schema/banco separado por tenant

Mesmo binário, mas cada tenant tem seu próprio schema Postgres (ou banco físico separado),
selecionado em runtime por configuração de conexão.

- **Isolamento**: melhor que B no nível de dados — um bug de query não cruza schema/banco (a query
  não vê a tabela do outro tenant). Ainda compartilha o processo, então bug de roteamento de tenant
  (escolher a connection string errada) ainda é possível, mas superfície menor que B.
- **Blast radius**: médio — isolado no dado, não no processo/CPU. Tenant com volume gigante pode
  saturar o pool compartilhado e degradar os outros (noisy neighbor).
- **Custo operacional**: médio-alto — migrations rodam N vezes (uma por schema/banco), automatizável,
  mas sem precisar de N deployments.
- **Onboarding**: mais pesado que B (criar schema + migration), mais leve que A.
- Bom quando há requisito de compliance explícito de separação física de dado por cliente.

## Design D — Namespace/processo separado por tenant, cluster compartilhado

Variante mais leve de A: isolamento de namespace Kubernetes com mesmo cluster/imagem, banco
compartilhado (volta a ser lógico, salvo combinação com C), mas processo de aplicação isolado por
tenant.

- **Isolamento**: força o processo a nunca ter dois tenants na mesma memória, mas o dado no banco
  continua isolamento lógico a menos que combinado com C.
- **Custo operacional**: menor que A puro (reusa cluster, CI/CD, observability), mas ainda cresce com
  número de tenants (N réplicas mínimas, N rotas de ingress).
- Escolhido quando o motivo de isolar é **blast radius de disponibilidade** (um tenant não deve
  afetar latência de outro), não dado.

## Comparação resumida

| | Isolamento | Blast radius (bug) | Custo op./tenant | Onboarding | Ruído entre vizinhos |
|---|---|---|---|---|---|
| A. Pod dedicado | Forte (processo+rede) | Mínimo | Alto, cresce linear | Pipeline de infra | Nenhum |
| B. Lógico (`tenant_id` + RLS) | Médio (reforçável) | Pode ser amplo se falhar | Mínimo | Insert numa tabela | Existe |
| C. Schema/DB por tenant | Forte no dado | Médio | Médio | Migration por schema | Existe |
| D. Namespace por tenant | Forte no processo | Médio (dado ainda exposto se banco compartilhado) | Médio-alto | Deploy por namespace | Reduzido |

## Conclusão da análise

O risco concreto ("garantir que tenant não receberia por outro") é fundamentalmente um problema de
**qual `handle` é enviado à InfinitePay na criação do link** e **qual `tenant_id` é usado pra
atribuir o webhook recebido** — isso é resolvido pela lógica de aplicação (resolver `handle` no
servidor, nunca do request; correlacionar webhook pelo nosso registro, não pelo payload), e essa
lógica **precisa existir em qualquer um dos quatro designs**. Pod dedicado por tenant não dispensa
essa lógica estar correta; só reduz o dano se ela falhar.

A escolha entre A/B/C/D é sobre **blast radius de um bug futuro e custo operacional**, não sobre se a
atribuição handle↔tenant vai estar certa.

Dado o estágio atual do projeto (mono-binário Go, um provider C6 só, sem infra multi-tenant hoje), o
caminho de menor risco de execução é **B com RLS no Postgres** como reforço, evoluindo para **C**
(schema por tenant) se/quando houver requisito de compliance explícito que exija separação física de
dado por cliente. **A** (pod por tenant) só se justificaria por um requisito de isolamento de
*disponibilidade* que B/C não resolvam — é o design mais caro de operar para um ganho de segurança
que a lógica de atribuição já deveria garantir.

**Status**: decisão ainda não tomada — discussão em andamento antes de comprometer o projeto a um
modelo multitenant.
