// One-off script: fills the 105 FORMTEXT fields of the C6 conformity
// roteiro (word/document.xml, already extracted to $TEMP/doc_raw.xml) with
// real evidence captured from tests/bruno/reports/latest.json and the
// underlying .bru request bodies. Field order/roles were mapped by hand by
// reading the document with indexed placeholders (see
// $TEMP/doc_with_fields.txt) -- 5 status fields (Em conformidade / Parcial /
// Erro / Nao se aplica / Nao foi testado) per top-level item, followed by
// one "Resposta teste" field per sub-item, repeating in document order.
//
// Each field's content is a list of "segments": plain prose strings
// (rendered in the field's original font/color) interleaved with parsed
// JSON values (rendered pretty-printed, multi-line, in Courier New with
// per-token syntax-highlight colors) for human readability.
const fs = require('fs');

function json(value) {
  return { __json: true, value };
}

const PROSE = {
  bankSlipsHead: (body) => `POST /v1/bank-slips com ${body} -> 201 Created. Resposta: `,
};

// ---------------------------------------------------------------------
// Field content map. Plain string => single plain run (org header values,
// status "X" marks, pure-prose explanations). Array => mixed prose/JSON
// segments, JSON rendered pretty-printed and colorized.
// ---------------------------------------------------------------------
const fields = {
  // Dados da organizacao -- 1 (empresa) and 5 (telefone) intentionally
  // left unset (untouched) for the user to fill in by hand.
  2: 'Banking',
  3: 'Leonardo Alves de Paula e Silva',
  4: 'leoapsilva@gmail.com',

  // Item 1 - Emissao de Boleto Simples -> Em conformidade
  6: 'X',
  11: [
    'POST /v1/bank-slips com interest{type:PERCENTAGE,value:1,deadline_days:0} e fine{type:PERCENTAGE,value:2,deadline_days:0} -> 201 Created. Resposta:',
    json({
      id: '01KVJD60NJT26AX4WV98CC3YKV',
      our_number: '10231722',
      bar_code: '33691167700000500000000065731330010231722213',
      digitable_line: '33690.00009   65731.330018   02317.222137   1   16770000050000',
      amount_cents: 50000,
      currency: 'BRL',
      due_date: '2026-12-31',
      status: 'CREATED',
    }),
  ],
  12: [
    'POST /v1/bank-slips sem interest/fine, apenas due_date -> 201 Created. Resposta:',
    json({
      id: '01KVJD5YV4JMXHXV5H0EQDJA6J',
      our_number: '10231721',
      bar_code: '33695167700000200000000065731330010231721213',
      digitable_line: '33690.00009   65731.330018   02317.212138   5   16770000020000',
      amount_cents: 20000,
      currency: 'BRL',
      due_date: '2026-12-31',
      status: 'CREATED',
    }),
  ],

  // Item 2 - Juros e Multa Variaveis -> Em conformidade
  13: 'X',
  18: [
    'POST /v1/bank-slips com interest{type:PERCENTAGE,value:1,deadline_days:0}, sem fine -> 201 Created. Resposta:',
    json({
      id: '01KVJDBVDF01BQPM716WNM7AFF',
      our_number: '10231724',
      amount_cents: 40000,
      due_date: '2026-12-31',
      status: 'CREATED',
    }),
  ],
  19: [
    'POST /v1/bank-slips com fine{type:PERCENTAGE,value:2,deadline_days:0}, sem interest -> 201 Created. Resposta:',
    json({
      id: '01KVJDBX92XPAHV6YZGHT66M73',
      our_number: '10231725',
      amount_cents: 40000,
      status: 'CREATED',
    }),
  ],
  20: [
    'POST /v1/bank-slips com fine{type:FIXED,value:500 (R$5,00),deadline_days:0} -> 201 Created. Resposta:',
    json({
      id: '01KVJDBZ46QWFTXDNNZQCQMNRF',
      our_number: '10231726',
      amount_cents: 40000,
      status: 'CREATED',
    }),
    'A API tambem aceita multa em percentual (validado no item 2.b); este caso confirma o tipo FIXED.',
  ],

  // Item 3 - Emissao com Desconto -> Em conformidade
  21: 'X',
  26: [
    'POST /v1/bank-slips com discount.tiers=[{type:PERCENTAGE,value:10,deadline_days:1}] (desconto unico se pago ate 1 dia antes do vencimento) -> 201 Created. Resposta:',
    json({
      id: '01KVJDC0ZAQXRHNAGNGMN36FM8',
      our_number: '10231727',
      amount_cents: 30000,
      status: 'CREATED',
    }),
  ],
  27: [
    'POST /v1/bank-slips com discount.tiers=[{10%,10 dias},{5%,5 dias},{2%,1 dia}] (desconto escalonado por dias antes do vencimento) -> 201 Created. Resposta:',
    json({
      id: '01KVJD62H9JXSGY4WFXKCCBG12',
      our_number: '10231723',
      amount_cents: 30000,
      status: 'CREATED',
    }),
    'Validacao complementar: tiers com deadline_days nao estritamente decrescente sao rejeitados pela nossa API com "discount tiers must have strictly decreasing deadline_days".',
  ],

  // Item 4 - Baixa de Boleto Emitido -> Em conformidade (4.b confirmado empiricamente)
  28: 'X',
  33: 'PUT /v1/bank-slips/{id}/cancel?baas=c6 imediatamente apos a emissao -> bloqueado pelo C6 com 400 "Evento nao pode ser realizado, pois ja existe uma requisicao a CIP sujeita a aprovacao" (regra de negocio esperada, registro inicial na CIP ainda pendente). Repetindo a baixa apos aguardar a confirmacao da CIP (~90s): 204 No Content. Conformidade confirmada para boleto ainda nao vencido.',
  34: [
    'O swagger do C6 documenta due_date >= data atual na criacao, e nossa API replica essa regra por padrao. Para testar genuinamente este item, desativamos temporariamente essa validacao apenas para um experimento controlado contra o sandbox: POST /v1/bank-slips com due_date no passado (ontem) -> 201 Created (o C6 ACEITOU, contradizendo a propria documentacao). Resposta:',
    json({
      id: '01KVJSSMFH3FM107P0VZ0SY18M',
      our_number: '10231785',
      bar_code: '33691148200000120000000065731330010231785213',
      digitable_line: '33690.00009   65731.330018   02317.852131   1   14820000012000',
      amount_cents: 12000,
      currency: 'BRL',
      due_date: '2026-06-19',
      status: 'CREATED',
    }),
    'Em seguida, apos aguardar a confirmacao da CIP, PUT /v1/bank-slips/{id}/cancel?baas=c6 sobre esse boleto genuinamente vencido -> 204 No Content. Conformidade confirmada com um boleto realmente vencido (nao mais uma aproximacao). A validacao de due_date no passado foi restaurada em nossa API imediatamente apos o experimento (devolvendo o erro client-side de novo, em vez de depender do C6 aceitar ou nao).',
  ],

  // Item 5 - Alteracao de Dados do Boleto -> Em conformidade (5.a e 5.b confirmados)
  35: 'X',
  40: [
    'O C6 nao documenta um campo separado para "data de validade" -- vencimento (5.a) e validade (5.b) sao o mesmo campo due_date; nossa interpretacao e que a distincao do roteiro e o tipo de boleto (com ou sem juros/multa). Testado aqui num boleto COM interest/fine: POST /v1/bank-slips -> 201 Created, depois PUT due_date (apos aguardar a CIP) -> 200 OK. Resposta da alteracao:',
    json({
      id: '01KVJSPS5AT65VWVS5S1TV5Q0R',
      external_reference_id: 'JMQMI42U8',
      status: 'CREATED',
      amount_cents: 45000,
      currency: 'BRL',
      due_date: '2027-02-01',
      our_number: '10231784',
      bar_code: '33699167700000450000000065731330010231784213',
      digitable_line: '33690.00009   65731.330018   02317.842132   9   16770000045000',
    }),
  ],
  41: [
    'Mesmo campo due_date, agora testado num boleto SEM interest/fine ("data limite de pagamento", como no item 1.b): PUT /v1/bank-slips/{id} alterando due_date para 2027-01-15 (apos aguardar a confirmacao da CIP) -> 200 OK. Resposta:',
    json({
      id: '01KVJD5YV4JMXHXV5H0EQDJA6J',
      external_reference_id: 'AMQMAAK9P',
      status: 'CREATED',
      amount_cents: 20000,
      currency: 'BRL',
      due_date: '2027-01-15',
      instructions: ['Nao receber apos o vencimento'],
      our_number: '10231721',
      bar_code: '33695167700000200000000065731330010231721213',
      digitable_line: '33690.00009   65731.330018   02317.212138   5   16770000020000',
    }),
  ],
  42: [
    'PUT /v1/bank-slips/{id} alterando amount_cents (25000->37500), fine (3%) e interest (2%) (apos aguardar a CIP) -> 200 OK. Resposta:',
    json({
      id: '01KVJDC2G39TXS5YKT0SVMQ4Q0',
      external_reference_id: 'HMQMAEVJF',
      status: 'CREATED',
      amount_cents: 37500,
      currency: 'BRL',
      due_date: '2026-12-31',
      our_number: '10231728',
      bar_code: '33696167700000250000000065731330010231728213',
      digitable_line: '33690.00009   65731.330018   02317.282131   6   16770000025000',
    }),
    'Observacao: o C6 nao ecoa fine/interest no corpo de retorno do PUT nem no GET subsequente (comportamento observado do sandbox); a validacao se baseia no amount_cents retornado e no status 200.',
  ],

  // Item 6 - Consulta de Boleto -> Em conformidade
  43: 'X',
  48: [
    'GET /v1/bank-slips/{id}?baas=c6 -> 200 OK. Resposta:',
    json({
      id: '01KVJD5YV4JMXHXV5H0EQDJA6J',
      external_reference_id: 'AMQMAAK9P',
      status: 'CREATED',
      amount_cents: 20000,
      currency: 'BRL',
      due_date: '2026-12-31',
      instructions: ['Nao receber apos o vencimento'],
      our_number: '10231721',
      bar_code: '33695167700000200000000065731330010231721213',
      digitable_line: '33690.00009   65731.330018   02317.212138   5   16770000020000',
    }),
  ],

  // Item 7 - Criar cobranca imediata (PIX) -> Em conformidade
  49: 'X',
  54: [
    'POST /v1/pix/charges com expiration_seconds:3600, sem devedor -> 201 Created. Resposta:',
    json({
      txid: 'QRS1TXU3M8PRFJQEYQV3TRHBIUPKTBHCLU4',
      kind: 'IMMEDIATE',
      status: 'ACTIVE',
      amount_cents: 10000,
      pix_key: '********',
      revision: 0,
      location: 'qrcode-h.c6pix.com/qrs1/v2/01P76VOrT6EOnlZYXUAngSqhRkmZgStYyNKUJns2JSoLPUsYZE',
      expiration_seconds: 3600,
    }),
  ],
  55: [
    'POST /v1/pix/charges com debtor{document_type:CPF,document_id:12345678909,name:"Jose da Silva"} -> 201 Created. Resposta:',
    json({
      txid: 'QRS2TXHQTT2UFA8DZFXWIIYSHNJRHPCB7VR',
      kind: 'IMMEDIATE',
      status: 'ACTIVE',
      amount_cents: 15000,
      pix_key: '********',
      revision: 0,
      location: 'qrcode-h.c6pix.com/qrs2/v2/02b8s1V1VcD5OQnPo5Wla3AIcbHHbmhn9pEsBq9PZtStOy1LCy',
      expiration_seconds: 3600,
    }),
  ],
  56: [
    'GET /v1/pix/charges/{txid}?baas=c6 (txid criado no passo 7.1) -> 200 OK. Resposta:',
    json({
      txid: 'QRS1TXU3M8PRFJQEYQV3TRHBIUPKTBHCLU4',
      kind: 'IMMEDIATE',
      status: 'ACTIVE',
      amount_cents: 10000,
      pix_key: '********',
      revision: 0,
      location: 'qrcode-h.c6pix.com/qrs1/v2/01P76VOrT6EOnlZYXUAngSqhRkmZgStYyNKUJns2JSoLPUsYZE',
      expiration_seconds: 3600,
    }),
  ],
  57: [
    'GET /v1/pix/charges?baas=c6&start=2026-01-01T00:00:00Z&end=2026-12-31T23:59:59Z -> 200 OK. Resposta com 17 cobrancas no periodo; amostra abaixo (2 das 17):',
    json({
      charges: [
        {
          txid: 'QRS1TXU3M8PRFJQEYQV3TRHBIUPKTBHCLU4',
          kind: 'IMMEDIATE',
          status: 'ACTIVE',
          amount_cents: 10000,
          revision: 0,
        },
        {
          txid: 'QRS2TXHQTT2UFA8DZFXWIIYSHNJRHPCB7VR',
          kind: 'IMMEDIATE',
          status: 'ACTIVE',
          amount_cents: 15000,
          revision: 0,
        },
      ],
    }),
  ],
  58: [
    'POST /v1/pix/charges com txid proprio, due_date:2026-12-31, days_valid_after_due_date:30 e debtor com endereco completo -> 201 Created. Resposta:',
    json({
      txid: 'BRUNOTXIDDUEDATE0000000000001',
      kind: 'WITH_DUE_DATE',
      status: 'ACTIVE',
      amount_cents: 25000,
      pix_key: '********',
      revision: 2,
      due_date: '2026-12-31',
    }),
  ],
  59: [
    'GET /v1/pix/charges?baas=c6&kind=with_due_date&start=2026-01-01T00:00:00Z&end=2026-12-31T23:59:59Z -> 200 OK. Resposta:',
    json({
      charges: [
        {
          txid: 'BRUNOTXIDDUEDATE0000000000001',
          kind: 'WITH_DUE_DATE',
          status: 'ACTIVE',
          amount_cents: 25000,
          revision: 1,
          due_date: '2026-12-31',
        },
      ],
    }),
  ],
  60: [
    'GET /v1/pix/charges/{txid}?baas=c6&kind=with_due_date (txid criado no passo 7.5) -> 200 OK. Resposta:',
    json({
      txid: 'BRUNOTXIDDUEDATE0000000000001',
      kind: 'WITH_DUE_DATE',
      status: 'ACTIVE',
      amount_cents: 25000,
      revision: 1,
      due_date: '2026-12-31',
    }),
  ],
  61: 'POST /v1/pix/webhook com pix_key e url de notificacao -> 200 OK. Webhook cadastrado com sucesso para a chave PIX da conta sandbox.',

  // Item 8 - Consulta DDA / Agendamento de pagamentos -> Em conformidade
  62: 'X',
  67: [
    'NOTA SOBRE O CAMPO "txid" PEDIDO EM TODOS OS ITENS DESTE BLOCO: verificamos o spec oficial (agendamento-de-pagamentos.yaml) e nao ha nenhuma ocorrencia de "txid" -- os identificadores reais desta API sao group_id e o id de cada item. txid e um conceito exclusivo do PIX (definido em pix.yaml). Tambem inspecionamos os headers HTTP crus devolvidos pelo C6 nessas chamadas (logging temporario, sem expor o Authorization) e nao ha nenhum header tipo txid -- so headers de infraestrutura (Cloudflare, X-Request-Id, Date, Server, Via). A frase "Status XXX com o retorno do txid" repetida em todas as celulas deste bloco parece ser um residuo de copy-paste do template usado no bloco de PIX (que usa txid de verdade) e nao foi adaptada para esta secao.',
    'GET /v1/payment-scheduling/dda?baas=c6 -> 200 OK. Resposta:',
    json({ items: [] }),
    '(sem boletos pendentes de pagamento cadastrados no DDA da conta sandbox no momento do teste).',
  ],
  68: [
    'POST /v1/payment-scheduling/groups com 3 itens PIX (identificados via content={{pix_key}}) -> 201 Created. Resposta:',
    json({ group_id: '01KVJDJC4CN953GC9NWJQHB6PQ' }),
    'Descoberta durante a validacao: o decode e assincrono no C6 -- o group_id e retornado de imediato, mas a decodificacao de cada item (validacao de chave PIX/boleto via DICT) ocorre em segundo plano; uma consulta aos itens imediatamente apos pode retornar 422 "ainda estao no processo de decodificacao". O adapter (internal/provider/c6/paymentscheduling_adapter.go) trata isso com retry/backoff de ate ~30s antes de propagar erro.',
  ],
  69: [
    'GET /v1/payment-scheduling/groups/{group_id}/items?baas=c6 -> 200 OK (apos o retry automatico aguardar a decodificacao). Resposta:',
    json({
      items: [
        {
          id: '01KVJDJC52NEZ26F2C5N6N4BS5',
          group_id: '01KVJDJC4CN953GC9NWJQHB6PQ',
          amount_cents: 2000,
          description: 'Item B - sera removido em lote',
          transaction_date: '2026-06-20',
          product_type: 'PIX',
          status: 'READ_DATA',
        },
        {
          id: '01KVJDJC591QPK0078F682SFRT',
          group_id: '01KVJDJC4CN953GC9NWJQHB6PQ',
          amount_cents: 3000,
          description: 'Item C - sera mantido e submetido para aprovacao',
          transaction_date: '2026-06-20',
          product_type: 'PIX',
          status: 'READ_DATA',
        },
        {
          id: '01KVJDJC4NX61ZY4DCVGWNCHA2',
          group_id: '01KVJDJC4CN953GC9NWJQHB6PQ',
          amount_cents: 1000,
          description: 'Item A - sera removido individualmente',
          transaction_date: '2026-06-20',
          product_type: 'PIX',
          status: 'READ_DATA',
        },
      ],
    }),
  ],
  70: 'DELETE /v1/payment-scheduling/groups/{group_id}/items com item_ids=[item_b] (remocao em lote, amount_cents=2000) -> 204 No Content.',
  71: 'DELETE /v1/payment-scheduling/groups/{group_id}/items/{item_id}?baas=c6 (item unico, amount_cents=1000) -> 204 No Content.',
  72: 'POST /v1/payment-scheduling/groups/{group_id}/submit com uploader_name:"Bruno Teste" (submetendo o item remanescente, amount_cents=3000) -> 204 No Content.',

  // Item 9 - Criar checkout -> Erro (401 do C6, credencial sem o produto habilitado)
  75: 'X',
  78: [
    'POST /v1/checkouts com card.type=CREDIT, installments=1, authenticate=NOT_REQUIRED -> 502 (erro repassado do provider). Corpo:',
    json({
      error:
        'checkout: create via provider: c6: unexpected status 401: {"type":"https://developers.c6bank.com.br/v1/error/unauthorized","title":"Nao autorizado.","status":401,"detail":"Consulte a documentacao.","correlation_id":"a0ea7bac17c7d7d4-GRU","timestamp":"2026-06-20T11:37:45.305Z"}',
    }),
    'Corpo de erro do C6 (decodificado da string acima):',
    json({
      type: 'https://developers.c6bank.com.br/v1/error/unauthorized',
      title: 'Nao autorizado.',
      status: 401,
      detail: 'Consulte a documentacao.',
      correlation_id: 'a0ea7bac17c7d7d4-GRU',
      timestamp: '2026-06-20T11:37:45.305Z',
    }),
    'O 401 vem diretamente do C6 (a mesma credencial mTLS+OAuth2 autentica normalmente para Boleto, PIX e DDA); indica que o produto Checkout/C6 Pay especificamente ainda nao esta habilitado para esta credencial -- pendente de confirmacao/ativacao pelo C6.',
  ],
  79: [
    'POST /v1/checkouts com card.type=DEBIT -> mesmo 401 do C6 (correlation_id a0ea7be79025d7d4-GRU). Ver detalhe completo no item 9.a; mesma causa raiz.',
  ],
  80: [
    'POST /v1/checkouts com card.authenticate=OPTIONAL -> mesmo 401 do C6 (correlation_id a0ea7c09951dd7d4-GRU). Ver detalhe completo no item 9.a; mesma causa raiz.',
  ],

  // Item 10 - Consultar checkout -> Erro (depende do item 9)
  83: 'X',
  86: [
    'GET /v1/checkouts/{id}?baas=c6 -- como nenhum checkout foi criado com sucesso no item 9 (401), a consulta foi exercitada contra um id de teste e retornou 400 do C6. Corpo de erro do C6 (decodificado):',
    json({
      type: 'https://developers.c6bank.com.br/v1/error/invalid_request',
      title: 'Requisicao invalida.',
      status: 400,
      detail: "transactionId '{{checkout_id}}' nao valido",
      correlation_id: 'a0ea7bbbf22ad7d4-GRU',
    }),
    'Rota e mapeamento implementados e validados estruturalmente; o teste de ponta a ponta depende da resolucao do bloqueio do item 9 junto ao C6.',
  ],

  // Item 11 - Cancelar checkout -> Erro (depende do item 9)
  89: 'X',
  92: [
    'PUT /v1/checkouts/{id}/cancel?baas=c6 -- mesma dependencia do item 10.a (nenhum checkout real disponivel). Corpo de erro do C6 (decodificado):',
    json({
      type: 'https://developers.c6bank.com.br/v1/error/invalid_request',
      title: 'Requisicao invalida.',
      status: 400,
      detail: "transactionId '{{checkout_id}}' nao valido",
      correlation_id: 'a0ea7bc5c363d7d4-GRU',
    }),
    'Rota e mapeamento implementados; teste de ponta a ponta depende da resolucao do item 9.',
  ],

  // Item 12 - Criar link com cartao de credito (Opcional) -> Parcial (12.a erro, 12.b nao testado)
  94: 'X',
  98: [
    'POST /v1/checkouts com card.save_card=true (tokenizacao) -> mesmo 401 do C6 (correlation_id a0ea7c1ce004d7d4-GRU). Ver detalhe completo no item 9.a; mesma causa raiz.',
  ],
  99: 'NAO TESTADO: a captura com token (POST /v1/checkouts/authorize) existe internamente no provider C6 (usada pelo worker de cobranca recorrente de assinaturas), mas nao esta exposta como rota HTTP publica na nossa API -- decisao de produto pendente sobre expo-la publicamente. Tambem depende de um token de cartao salvo, que so seria obtido com sucesso no item 12.a, hoje bloqueado pelo 401.',

  // Item 13 - Configurar webhook do checkout -> Nao foi testado (registro real nunca executado)
  104: 'X',
  105: 'NAO TESTADO contra o C6 real: o registro de webhook (POST /v1/webhooks/register, service=CHECKOUT) tem efeito colateral persistente na configuracao da conta sandbox, por isso e deliberadamente excluido da execucao padrao da suite de testes (requer a flag --include-webhook-register). O recebimento (inbound) de notificacoes do C6 foi validado de forma simulada e idempotente (2/2 passando localmente), mas o registro outbound real nunca foi executado contra o C6 nesta validacao.',
};

// ---------------------------------------------------------------------
// JSON tokenizer/colorizer: walks a JS value directly (rather than
// re-parsing JSON.stringify output) and emits typed tokens so each piece
// can be rendered in its own colored run.
// ---------------------------------------------------------------------
const COLORS = {
  punct: '444444',
  key: '0451A5',
  string: 'A31515',
  number: '098658',
  bool: 'AF00DB',
  null: 'AF00DB',
};

function tokenizeJson(value) {
  const tokens = []; // {type:'punct'|'key'|'string'|'number'|'bool'|'null'|'ws'|'nl', text}
  const IND = '    ';
  const pad = (n) => IND.repeat(n);
  const emit = (type, text) => tokens.push({ type, text });

  function walk(v, depth) {
    if (v === null) {
      emit('null', 'null');
      return;
    }
    if (typeof v === 'boolean') {
      emit('bool', String(v));
      return;
    }
    if (typeof v === 'number') {
      emit('number', String(v));
      return;
    }
    if (typeof v === 'string') {
      emit('string', JSON.stringify(v));
      return;
    }
    if (Array.isArray(v)) {
      if (v.length === 0) {
        emit('punct', '[]');
        return;
      }
      emit('punct', '[');
      emit('nl', '');
      v.forEach((item, i) => {
        emit('ws', pad(depth + 1));
        walk(item, depth + 1);
        if (i < v.length - 1) emit('punct', ',');
        emit('nl', '');
      });
      emit('ws', pad(depth));
      emit('punct', ']');
      return;
    }
    const keys = Object.keys(v);
    if (keys.length === 0) {
      emit('punct', '{}');
      return;
    }
    emit('punct', '{');
    emit('nl', '');
    keys.forEach((k, i) => {
      emit('ws', pad(depth + 1));
      emit('key', JSON.stringify(k));
      emit('punct', ': ');
      walk(v[k], depth + 1);
      if (i < keys.length - 1) emit('punct', ',');
      emit('nl', '');
    });
    emit('ws', pad(depth));
    emit('punct', '}');
  }

  walk(value, 0);
  return tokens;
}

function escapeXml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// Builds the OOXML run sequence for one field's content, given the rPr
// block copied from the field's original (placeholder) run, so prose
// segments keep the form's normal font/size and JSON segments switch to
// Courier New with per-token colors.
function buildRuns(content, baseRPr) {
  const segments = Array.isArray(content) ? content : [content];
  let xml = '';

  function proseRun(text) {
    if (!text) return;
    xml += `<w:r>${baseRPr}<w:t xml:space="preserve">${escapeXml(text)}</w:t></w:r>`;
    // line break between a prose segment and the next segment, so JSON
    // blocks always start on their own line
    xml += '<w:r><w:br/></w:r>';
  }

  function jsonRun(value) {
    const tokens = tokenizeJson(value);
    for (const tok of tokens) {
      if (tok.type === 'nl') {
        xml += '<w:r><w:br/></w:r>';
        continue;
      }
      const color = COLORS[tok.type];
      const colorTag = color ? `<w:color w:val="${color}"/>` : '';
      const rPr = `<w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New" w:cs="Courier New"/>${colorTag}<w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>`;
      xml += `<w:r>${rPr}<w:t xml:space="preserve">${escapeXml(tok.text)}</w:t></w:r>`;
    }
    xml += '<w:r><w:br/></w:r>';
  }

  segments.forEach((seg) => {
    if (seg && seg.__json) {
      jsonRun(seg.value);
    } else {
      proseRun(seg);
    }
  });

  // Trailing <w:br/> from the loop is redundant with the field boundary;
  // strip exactly one trailing break run to avoid a dangling blank line.
  xml = xml.replace(/<w:r><w:br\/><\/w:r>$/, '');
  return xml;
}

const raw = fs.readFileSync(process.argv[2] || (process.env.TEMP + '/doc_raw.xml'), 'utf8');

// ---------------------------------------------------------------------
// "Informar txid:" banners: these are NOT FORMTEXT fields (no fldChar) --
// just a bold static-text row the C6 template inserts between PIX
// sub-items (and, evidently copy-pasted without adapting, between every
// DDA sub-item too, where there is no txid concept at all -- see field 67
// above). Since there's no field to target, the txid value has to be
// inserted as a new plain run appended right after the label, before the
// paragraph closes. Positions are byte offsets into the ORIGINAL raw doc
// (found by manual inspection), applied back-to-front so each insertion
// doesn't invalidate the positions still to come.
const txidBanners = [
  { pos: 316438, value: 'QRS1TXU3M8PRFJQEYQV3TRHBIUPKTBHCLU4' }, // 7.1
  { pos: 323148, value: 'QRS2TXHQTT2UFA8DZFXWIIYSHNJRHPCB7VR' }, // 7.2
  { pos: 344280, value: 'BRUNOTXIDDUEDATE0000000000001' }, // 7.5
  { pos: 399503, value: 'Nao se aplica -- API de DDA usa group_id/item id, nao txid (ver nota no item 8.1).' }, // 8.1
  { pos: 406600, value: 'Nao se aplica -- ver nota no item 8.1.' }, // 8.2
  { pos: 414146, value: 'Nao se aplica -- ver nota no item 8.1.' }, // 8.3
  { pos: 421941, value: 'Nao se aplica -- ver nota no item 8.1.' }, // 8.4
  { pos: 429711, value: 'Nao se aplica -- ver nota no item 8.1.' }, // 8.5
  { pos: 437676, value: 'Nao se aplica -- ver nota no item 8.1.' }, // 8.6
];

let rawWithTxids = raw;
for (const { pos, value } of [...txidBanners].sort((a, b) => b.pos - a.pos)) {
  const closeIdx = rawWithTxids.indexOf('</w:p>', pos);
  if (closeIdx === -1) throw new Error('txid banner at ' + pos + ': no closing </w:p> found');
  const newRun = `<w:r><w:rPr><w:rFonts w:ascii="Urbanist" w:hAnsi="Urbanist"/><w:snapToGrid w:val="0"/><w:color w:val="000000"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr><w:t xml:space="preserve"> ${escapeXml(value)}</w:t></w:r>`;
  rawWithTxids = rawWithTxids.slice(0, closeIdx) + newRun + rawWithTxids.slice(closeIdx);
}

const blockPattern = /<w:fldChar w:fldCharType="begin">[\s\S]*?<\/w:fldChar>[\s\S]*?<w:fldChar w:fldCharType="end"\/>/g;

let fieldIndex = 0;
let replacedCount = 0;
const result = rawWithTxids.replace(blockPattern, (block) => {
  fieldIndex++;
  const content = fields[fieldIndex];
  if (content === undefined) return block; // leave untouched (incl. org header 1 and 5)

  const sepMarker = '<w:fldChar w:fldCharType="separate"/>';
  const sepIdx = block.indexOf(sepMarker);
  if (sepIdx === -1) throw new Error('field ' + fieldIndex + ': no separate marker');
  const sepRunEndIdx = block.indexOf('</w:r>', sepIdx) + '</w:r>'.length;

  const endMarker = '<w:fldChar w:fldCharType="end"/>';
  const endIdx = block.lastIndexOf(endMarker);
  // NB: searching for '<w:r' would wrongly match inside '<w:rPr>' (which
  // starts with the same 4 chars) -- anchor to the actual run-open tag.
  const runOpenPattern = /<w:r(?=[ >])/g;
  let m;
  let endRunStartIdx = -1;
  while ((m = runOpenPattern.exec(block.slice(0, endIdx)))) {
    endRunStartIdx = m.index;
  }
  if (endRunStartIdx === -1) throw new Error('field ' + fieldIndex + ': no run-open tag found before end marker');

  const head = block.slice(0, sepRunEndIdx);
  const middle = block.slice(sepRunEndIdx, endRunStartIdx);
  const tail = block.slice(endRunStartIdx);

  const rPrMatch = middle.match(/<w:rPr>[\s\S]*?<\/w:rPr>/);
  const baseRPr = rPrMatch ? rPrMatch[0] : '';

  const newRuns = buildRuns(content, baseRPr);

  replacedCount++;
  return head + newRuns + tail;
});

if (fieldIndex !== 105) throw new Error('expected 105 fields, found ' + fieldIndex);

console.log('Fields replaced: ' + replacedCount + ' / ' + fieldIndex + ' total fields found');
console.log('Fields left untouched (org header, user-filled): ' + (fieldIndex - replacedCount));

fs.writeFileSync(process.argv[3] || (process.env.TEMP + '/doc_transformed.xml'), result, 'utf8');
