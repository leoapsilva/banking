#!/usr/bin/env node
// Formats the JSON output of `bru run --reporter-json` into a human
// readable Markdown summary: pass/fail counts overall and per folder, plus
// a detailed list of every failing request with its assertion errors.
'use strict';

const fs = require('fs');

const [, , inputPath, outputPath] = process.argv;
if (!inputPath || !outputPath) {
  console.error('usage: format-report.js <input.json> <output.md>');
  process.exit(1);
}

const data = JSON.parse(fs.readFileSync(inputPath, 'utf8'));

const results = [];
for (const iteration of data) {
  for (const r of iteration.results || []) {
    results.push(r);
  }
}

function folderOf(p) {
  const norm = (p || '').replace(/\\/g, '/');
  const parts = norm.split('/');
  return parts.length > 1 ? parts[0] : '(raiz)';
}

let totalRequests = results.length;
let passedRequests = 0;
let failedRequests = 0;
let totalAssertions = 0;
let passedAssertions = 0;
let failedAssertions = 0;
let totalTests = 0;
let passedTests = 0;
let failedTests = 0;

const byFolder = new Map();
const failures = [];

for (const r of results) {
  const folder = folderOf(r.path);
  if (!byFolder.has(folder)) {
    byFolder.set(folder, { total: 0, passed: 0, failed: 0 });
  }
  const f = byFolder.get(folder);
  f.total++;

  const assertions = r.assertionResults || [];
  const failedAssertionsHere = assertions.filter((a) => a.status === 'fail');

  // `tests { }` blocks (chai-style expect/test) report through
  // testResults, separate from assert-block assertionResults. A request
  // using either mechanism can fail the overall request.
  const tests = r.testResults || [];
  const failedTestsHere = tests.filter((t) => t.status === 'fail');

  const reqFailed =
    r.status === 'fail' || failedAssertionsHere.length > 0 || failedTestsHere.length > 0;

  if (reqFailed) {
    failedRequests++;
    f.failed++;
  } else {
    passedRequests++;
    f.passed++;
  }

  for (const a of assertions) {
    totalAssertions++;
    if (a.status === 'pass') passedAssertions++;
    else failedAssertions++;
  }

  for (const t of tests) {
    totalTests++;
    if (t.status === 'pass') passedTests++;
    else failedTests++;
  }

  if (reqFailed) {
    failures.push({
      path: r.path,
      name: r.name,
      responseStatus: r.response && r.response.status,
      responseBody: r.response && r.response.data,
      runtimeError: r.error,
      failedAssertions: failedAssertionsHere,
      failedTests: failedTestsHere,
    });
  }
}

const now = new Date().toISOString();
const overall = failedRequests === 0 ? 'PASS' : 'FAIL';

let md = '';
md += `# Relatorio de testes de integracao - Banking API (Bruno)\n\n`;
md += `Gerado em: ${now}\n\n`;
md += `## Resumo\n\n`;
md += `| Metrica | Resultado |\n`;
md += `|---|---|\n`;
md += `| Status geral | **${overall}** |\n`;
md += `| Requests | ${totalRequests} (${passedRequests} passou, ${failedRequests} falhou) |\n`;
md += `| Assertions | ${passedAssertions}/${totalAssertions} |\n`;
if (totalTests > 0) {
  md += `| Tests (tests{} block) | ${passedTests}/${totalTests} |\n`;
}
md += `\n`;

md += `## Por pasta\n\n`;
md += `| Pasta | Total | Passou | Falhou |\n`;
md += `|---|---|---|---|\n`;
for (const [folder, f] of byFolder) {
  md += `| ${folder} | ${f.total} | ${f.passed} | ${f.failed} |\n`;
}
md += `\n`;

if (failures.length > 0) {
  md += `## Falhas (${failures.length})\n\n`;
  for (const f of failures) {
    md += `### ${f.path}\n\n`;
    md += `- Nome: ${f.name}\n`;
    if (f.responseStatus !== undefined) {
      md += `- Status HTTP da resposta: ${f.responseStatus}\n`;
    }
    if (f.runtimeError) {
      md += `- Erro de execucao: ${f.runtimeError}\n`;
    }
    if (f.responseBody !== undefined) {
      const bodyStr =
        typeof f.responseBody === 'string'
          ? f.responseBody
          : JSON.stringify(f.responseBody);
      md += `- Corpo da resposta: \`${bodyStr.slice(0, 500)}\`\n`;
    }
    if (f.failedAssertions.length > 0) {
      md += `- Assertions que falharam:\n`;
      for (const a of f.failedAssertions) {
        md += `  - \`${a.lhsExpr} ${a.operator} ${a.rhsExpr}\`: ${a.error}\n`;
      }
    }
    if (f.failedTests && f.failedTests.length > 0) {
      md += `- Tests que falharam:\n`;
      for (const t of f.failedTests) {
        md += `  - \`${t.description}\`: ${t.error || 'falhou'}\n`;
      }
    }
    md += `\n`;
  }
} else {
  md += `## Falhas\n\nNenhuma falha.\n`;
}

fs.writeFileSync(outputPath, md, 'utf8');
console.log(`Relatorio escrito em ${outputPath}`);
console.log(
  `Requests: ${totalRequests} (${passedRequests} passou, ${failedRequests} falhou) | Assertions: ${passedAssertions}/${totalAssertions}` +
    (totalTests > 0 ? ` | Tests: ${passedTests}/${totalTests}` : '')
);
