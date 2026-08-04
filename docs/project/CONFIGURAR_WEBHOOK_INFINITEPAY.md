# Configurar o webhook da InfinitePay — passo a passo

> Como o banking é avisado de que um pagamento foi confirmado. Sem isso, a
> compradora paga e o sistema nunca fica sabendo: o checkout permanece em
> `CREATED` e a assinatura nunca ativa.

## O que é, em uma frase

`INFINITEPAY_WEBHOOK_URL_TOKEN` é um **token que você inventa** e que vira o
último pedaço da rota que o serviço registra; `INFINITEPAY_WEBHOOK_URL` é essa
mesma rota escrita como endereço público, que informamos à InfinitePay a cada
cobrança criada.

```
token = a1b2c3d4e5f6...

  rota registrada:  POST /webhooks/infinitepay/a1b2c3d4e5f6...
  URL informada:    https://seu-dominio.com.br/webhooks/infinitepay/a1b2c3d4e5f6...
                                                                   └─ o mesmo token
```

**Não é uma credencial obtida no painel da InfinitePay.** Esse é o mal-entendido
mais comum, e o nome da variável ajuda a criá-lo (ver §Nota sobre o nome).

## Por que existe

O banking não é exposto à internet: escuta em `127.0.0.1` e o backend do Cores
fala com ele pela rede Docker interna. A única exceção é `/webhooks/*`, porque
a InfinitePay precisa alcançar o serviço de fora.

O token deixa essa rota difícil de adivinhar. É obscuridade, não autenticação —
um token na URL aparece em log de proxy. A defesa real contra notificação
forjada é a validação de valor em `internal/billing/service/webhook_validation.go`:
um evento `PAID` com valor abaixo do checkout é recusado.

---

## Passo a passo

### 1. Gerar o token — **você**

```bash
openssl rand -hex 16
```

Guarde junto dos outros segredos de produção. **Não improvise na hora do
deploy**: uma vez em uso, o valor não é facilmente trocável (ver §Rotação).

### 2. Preencher o `.env` da VPS — **você**

No `banking/.env` (o da raiz, que alimenta o processo — não o `docker/.env`):

```bash
INFINITEPAY_WEBHOOK_URL_TOKEN=<token gerado no passo 1>
INFINITEPAY_WEBHOOK_URL=https://seu-dominio.com.br/webhooks/infinitepay/<mesmo token>
```

Os dois valores têm de casar. Se não casarem, **o serviço não sobe** — a
validação foi adicionada exatamente para impedir que o erro apareça só quando
alguém já tiver pago.

### 3. Encaminhar o caminho no reverse proxy — **você**

O proxy do host precisa levar apenas o webhook para dentro do container:

```
/webhooks/*   →   127.0.0.1:8082
```

Nada além disso deve ser encaminhado. O resto da API é acessível só pela rede
`cores-shared`.

### 4. Preencher handle e base URL — **você**

```bash
# InfiniteTag da conta que recebe. Pode escrever com ou sem o '$' —
# o código remove o prefixo.
INFINITEPAY_HANDLE=cores-app-br

# Apenas o host. NÃO inclua /links: esse caminho é acrescentado pelo código
# (o client monta baseURL + "/links"), então uma base terminada em /links
# geraria /links/links. Pode deixar vazio — este é o valor padrão.
INFINITEPAY_BASE_URL=https://api.checkout.infinitepay.io
```

O endpoint efetivo de criação de cobrança é `POST https://api.checkout.infinitepay.io/links`,
mas a variável guarda **só a parte antes de `/links`**. O startup recusa uma
base que já inclua o caminho.

### 5. Subir e conferir — **você**

```bash
cd ~/banking/docker && docker-compose up -d
curl -sf http://127.0.0.1:8082/healthz && echo OK
```

Se o serviço não subir por causa das variáveis acima, a mensagem de erro diz
exatamente o que corrigir.

### 6. Testar o caminho de fora — **você**

De uma máquina externa, confirme que o caminho chega e que o resto não:

```bash
curl -i https://seu-dominio.com.br/webhooks/infinitepay/<token>   # deve responder (não 404 de proxy)
curl -i https://seu-dominio.com.br/v1/subscriptions               # deve falhar — não é para estar exposto
```

### 7. Validar com um pagamento real de ponta a ponta — **você**

Criar um checkout, pagar em sandbox e confirmar que o status vira `PAID`. É o
único teste que prova que a cadeia inteira funciona.

---

## O que já está feito — **eu**

| Item | Onde |
|---|---|
| Rota do webhook registrada com o token | `internal/webhook/handler/handler.go` |
| Envio da `webhook_url` em cada `POST /links` | `internal/provider/infinitepay/mapper/checkout.go` |
| Parse e dedup do evento recebido | `internal/webhook/service/service.go` |
| Recusa de `PAID` com valor abaixo do checkout | `internal/billing/service/webhook_validation.go` |
| Validação no startup de que URL e token casam | `internal/platform/config/config.go` (8 testes) |
| Documentação das variáveis | `.env.example` |
| Deploy falha se a porta for publicada em `0.0.0.0` | `.github/workflows/deploy.yml` |

Validações de configuração que falham o startup, todas com mensagem dizendo o
que corrigir:

| Erro | Por que é grave |
|---|---|
| `WEBHOOK_URL` não termina com o token | InfinitePay postaria numa rota inexistente; o pagamento entra e a assinatura nunca ativa |
| Só um dos dois preenchido | mesma consequência |
| `BASE_URL` terminando em `/links` | viraria `/links/links` — 404 na criação da cobrança |
| `API_KEYS` vazio (checado no deploy) | serviço subiria com todas as rotas anônimas |

## O que ainda não está feito

- **Ownership por API key.** `httpserver.Caller` já leva a identidade do
  chamador no contexto, mas nenhum handler a usa: quem tiver chave válida
  ainda cancela qualquer assinatura pelo UUID.
- **Backup do banco** ([#8](https://github.com/leoapsilva/banking/issues/8)) e
  **configuração do golangci-lint** ([#9](https://github.com/leoapsilva/banking/issues/9)).
- **Nada foi testado contra a InfinitePay real** — só build, vet e testes
  unitários. O passo 7 abaixo é o que prova a cadeia.

---

## Rotação

Trocar o token **quebra os checkouts em aberto**: cada cobrança já criada levou
a URL antiga, e a InfinitePay não tem como ser avisada retroativamente. Quem
pagar depois da troca recebe 404 e o pagamento se perde silenciosamente.

Se precisar rotacionar (vazamento, por exemplo), o caminho seguro é manter as
duas rotas ativas durante uma janela que cubra a validade dos links pendentes,
e só então remover a antiga. Isso exige mudança no código — hoje só uma rota é
registrada.

Vale lembrar que vazamento do token, isoladamente, não permite ativar
assinatura de graça: a validação de valor barra o evento forjado.

## Nota sobre o nome (renomeado em ago/2026)

A variável se chamava `INFINITEPAY_WEBHOOK_PATH_SECRET` — e o nome era a
origem de boa parte da confusão:

| O que o nome antigo sugeria | O que de fato é |
|---|---|
| caminho para um arquivo de segredo (`_PATH`, como em `C6_MTLS_CERT_PATH`) | um segmento da URL |
| chave criptográfica, de assinatura (`_SECRET`) | token de obscuridade, trafega em claro |
| algo obtido do provedor | valor que você mesmo gera |
| descartável, por ser aleatório | durável — a InfinitePay já guardou o valor |

`_URL_TOKEN` diz as duas coisas que importam: é um **token**, e ele vive na
**URL**. O par do C6 foi renomeado junto (`C6_WEBHOOK_PATH_SECRET` →
`C6_WEBHOOK_URL_TOKEN`) para não deixar o mesmo termo com dois sentidos.

⚠️ **Ao atualizar a VPS**, renomeie as chaves no `.env` — os valores em si não
mudam. Como o `.env` de produção não passa pelo CI, essa edição é manual.
