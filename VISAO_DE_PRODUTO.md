# Banking

Este projeto tem como objetivo ser um wrapper para todas as API de banking vamos oferecer para integrar com nossas aplicações:

- Boletos: API para emissão, consulta, atualização e download de boletos bancários.
- Bolepix: API para emissão, consulta, atualização e download de boletos de cobrança integrados com PIX
- Pix: API para criação e gerenciamento de cobranças PIX.
- Pix automático: API para criação, automação e gerenciamento de cobranças recorrentes via PIX.
- Checkout de pagamento: Soluções para pagamentos com cartão de crédito e débito, incluindo tokenização e recorrência.
- Notificações: API para recebimento automático de notificações de eventos através de webhooks.
- Transações e Recebíveis: API para consulta detalhada de transações, recebíveis, ajustes e chargebacks do C6 Pay.

# Premissas

- Uma API para cada tipo de serviço bancário 
- O cliente escolhe qual BaaS vai usar no payload
- O cliente sempre vai usar as mesmas rotas e payload 
- Existe uma camada de adaptção que vai traduzir o payload da API para o payload do BaaS destino e response também

# Prioridades

Até 30/06 deve ser possível pagar com cartão com recorrência:
- autenticação
- pagamento por cartão com recorrência (assinatura mensal)
- pagamento por cartão parcelado com juros assumidos pelo cliente (assinatura anual)
- cancelar pagamento por cartão com recorrecia (assinatura) 
- notificações 

# Arquitetura 

Avalie se esta é a melhor arquitetura e organização para a esturtura do projeto.

Eu acho que é a "Feature-Based Folder Structure".

Caso não seja, avalie dentre as existentes e sugeridas neste [artigo](https://medium.com/@smart_byte_labs/organize-like-a-pro-a-simple-guide-to-go-project-folder-structures-e85e9c1769c2).

## Feature-Based Folder Structure:
> In a feature-based folder structure, each feature or functionality of the application is treated as a separate unit. All code related to that feature, including handlers, services, repositories, and more, resides within that feature’s directory. This approach enhances cohesion within features and promotes the encapsulation of feature-specific logic.

### Estrutura do projeto

```
project/
├── cmd/
│   └── app/
│       └── main.go      # Main application logic
├── internal/
│   ├── user/            # Feature: User
│   │   ├── handler/      # User-specific HTTP Handlers
│   │   ├── service/      # User-specific Business Logic
│   │   ├── repository/   # User-specific Data Access
│   │   └── user.go       # User Model
│   ├── product/         # Feature: Product
│   │   ├── handler/      # Product-specific HTTP Handlers
│   │   ├── service/      # Product-specific Business Logic
│   │   ├── repository/   # Product-specific Data Access
│   │   └── product.go    # Product Model
│   ├── order/           # Feature: Order
│   │   ├── handler/      # Order-specific HTTP Handlers
│   │   ├── service/      # Order-specific Business Logic
│   │   ├── repository/   # Order-specific Data Access
│   │   └── order.go      # Order Model
├── pkg/                 # Shared utilities or helpers
│   └── logger.go        # Logging utilities
├── configs/             # Configuration files
├── go.mod               # Go module definition
└── go.sum               # Go module checksum file
```

# Bancos Integrados

## C6 Bank

Leia as documentações swagger de cada um dos tipos de serviços oferecidos priorizando:
- [autenticação](docs\c6\swagger\autenticação.yaml)
- [checkout](docs\c6\swagger\checkout-c6-bank.yaml)
- [notificações](docs\c6\swagger\notificações.yaml)

Leia a documentação de [homologação com o C6](docs\c6\homologacao\Roteiro conformidade_Boleto_PIX_Pagamentos_Checkout.docx) e faça um planejamento de implementação.

Já temos as chaves para acesso a conta para realização da homologação.