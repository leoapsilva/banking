// One-off: builds the final reply to C6 support (Endpoint/Ambiente/Client
// ID/cURL envio/cURL retorno per call) from the raw curl-repro output, and
// writes it straight to disk -- never printed to stdout, so the real
// client_secret/bearer token never flow back into the assistant's context.
const fs = require('fs');

const raw = fs.readFileSync(process.argv[2], 'utf8');

const header = `# Resposta ao suporte C6 (homologacaoapi@c6bank.com) -- evidencias cURL diretas

**NUNCA COMMITAR ESTE ARQUIVO** (esta pasta ja esta no .gitignore, mas confirme
antes de copiar trechos para fora dela). Contem o client_secret e bearer
tokens reais da credencial de sandbox.

Ambiente: sandbox
Client ID: a8bbb040-3f6d-48b3-97ae-9c079e065e38

Texto sugerido para a resposta ao e-mail:

---

Ola,

Conforme solicitado, seguem as evidencias com cURL direto contra os
endpoints do C6 (sem nenhuma camada nossa no meio), incluindo a chamada de
autenticacao (para confirmar que o token obtido tem os scopes
checkout.write/checkout.read/checkout.cancel) e as 4 variantes de criacao de
checkout que reproduzem o 401, em sandbox, com o Client ID acima.

---

`;

fs.writeFileSync(process.argv[3], header + raw, 'utf8');
console.log('Reply written to', process.argv[3], '(', fs.statSync(process.argv[3]).size, 'bytes)');
