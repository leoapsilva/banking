# Configurar o webhook da InfinitePay — passo a passo

> Como o banking é avisado de que um pagamento foi confirmado. Sem isso, a
> compradora paga e o sistema nunca fica sabendo: o checkout permanece em
> `CREATED` e a assinatura nunca ativa.

## O que é, em uma frase

`INFINITEPAY_WEBHOOK_PATH_SECRET` é um **token que você inventa** e que vira o
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
INFINITEPAY_WEBHOOK_PATH_SECRET=<token gerado no passo 1>
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

### 4. Preencher o handle da InfinitePay — **você**

```bash
INFINITEPAY_HANDLE=<sua InfiniteTag, sem o $>
```

É a credencial que identifica a conta que recebe. `INFINITEPAY_BASE_URL` pode
ficar vazia (o código usa `https://api.checkout.infinitepay.io` por padrão).

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

## O que ainda não está feito — **eu, quando você decidir**

- **Renomear a variável.** `PATH_SECRET` é ambíguo: no mesmo `.env`, `_PATH`
  significa caminho de arquivo (`C6_MTLS_CERT_PATH`), e `SECRET` sugere chave
  criptográfica, que não é. `INFINITEPAY_WEBHOOK_URL_TOKEN` descreveria melhor.
  Vale para `C6_WEBHOOK_PATH_SECRET` também. Barato agora, caro depois de
  estar em produção.
- **Confirmar a `INFINITEPAY_BASE_URL`.** O valor está no nosso código, mas não
  encontrei onde a InfinitePay o documenta — a página de desenvolvedores não
  publica referência de API. Um `POST /links` em sandbox confirma.

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

## Nota sobre o nome

`INFINITEPAY_WEBHOOK_PATH_SECRET` descreve mal o que a variável é:

| O que o nome sugere | O que de fato é |
|---|---|
| caminho para um arquivo de segredo | um segmento da URL |
| chave criptográfica (assinatura/HMAC) | token de obscuridade, trafega em claro |
| algo obtido do provedor | valor que você mesmo gera |
| descartável, por ser aleatório | durável — a InfinitePay já guardou o valor |
