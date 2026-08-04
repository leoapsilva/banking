// One-off: reads the raw curl-repro output (which contains the real
// client_secret and bearer token) and produces a redacted version safe to
// display in chat/logs. The full unredacted version is written separately
// to the docs file meant for the actual email reply to C6 support.
const fs = require('fs');

const raw = fs.readFileSync(process.argv[2], 'utf8');

// Bearer tokens here are base64-ish but may include +, /, = and other
// non-word chars; redact everything between "Bearer " and the closing
// single-quote of the curl -H argument rather than guessing a charset.
const redacted = raw
  .replace(/client_secret=[^&'\s]+/g, 'client_secret=<REDACTED>')
  .replace(/Bearer [^']+/g, 'Bearer <REDACTED_TOKEN>')
  .replace(/"access_token":"[^"]+"/g, '"access_token":"<REDACTED>"');

fs.writeFileSync(process.argv[3], redacted, 'utf8');
console.log('Redacted version written to', process.argv[3]);
