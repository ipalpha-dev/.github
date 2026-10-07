import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { completeLocalEnv, readEnv } from '../lib/local-env.mjs';

const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-env-test-'));
const fallback = path.join(tooling, 'templates/env-fallback');
const envFile = (name) => path.join(root, name === 'forms-api' ? 'apps/forms/forms-api/.env' : `core/${name}/.env`);
const write = (file, text) => { fs.mkdirSync(path.dirname(file), { recursive: true }); fs.writeFileSync(file, text); };
const shell = (script) => execFileSync('/bin/bash', ['-c', `set -euo pipefail
source lib/common.sh
source lib/i18n.sh
source lib/env.sh
source lib/settings.sh
source lib/ports.sh
${script}`], { cwd: tooling, encoding: 'utf8', env: { ...process.env, IPALPHA_TEST_NO_PORT_PROBE: '1' } });
try {
  write(envFile('auth-api'), fs.readFileSync(path.join(fallback, 'auth-api.env'), 'utf8'));
  write(envFile('persons-api'), 'AUTH_TOKEN_ISSUER=""\nIMPORT_ROWS_KEY=\nAUTH_CLIENT_SECRET=operator-secret-long-enough\nCUSTOM_VALUE=keep-me\nAI_API_URL=http://localhost:3010\n');
  completeLocalEnv(path.dirname(envFile('persons-api')), 'persons-api', path.join(fallback, 'persons-api.env'));
  let persons = readEnv(envFile('persons-api'));
  assert.equal(persons.AUTH_TOKEN_ISSUER, 'http://localhost:3005');
  assert.ok(persons.INSTANCE_ID);
  assert.equal(persons.FORMS_RESPONDENT_ENTRY_POINT_ID, 'ep-ipalpha-forms-respondent');
  assert.equal(Buffer.from(persons.IMPORT_ROWS_KEY, 'base64').length, 32);
  assert.equal(persons.AUTH_CLIENT_SECRET, 'operator-secret-long-enough');
  assert.equal(persons.CUSTOM_VALUE, 'keep-me');
  assert.equal(persons.AI_API_URL, 'http://127.0.0.1:3008');
  const encryptionKey = persons.IMPORT_ROWS_KEY;
  completeLocalEnv(path.dirname(envFile('persons-api')), 'persons-api', path.join(fallback, 'persons-api.env'));
  assert.equal(readEnv(envFile('persons-api')).IMPORT_ROWS_KEY, encryptionKey);
  assert.equal(fs.statSync(envFile('persons-api')).mode & 0o777, 0o600);
  write(envFile('forms-api'), 'AUTH_TOKEN_ISSUER=https://auth.operator.invalid\nFORMS_APP_ID=custom-app\n');
  completeLocalEnv(path.dirname(envFile('forms-api')), 'forms-api', path.join(fallback, 'forms-api.env'));
  assert.equal(readEnv(envFile('forms-api')).FORMS_APP_ID, 'custom-app');
  assert.equal(readEnv(envFile('forms-api')).AUTH_TOKEN_ISSUER, 'https://auth.operator.invalid');
  fs.appendFileSync(envFile('forms-api'), 'AUTH_CLIENT_ID=""\nAUTH_CLIENT_SECRET=\'\'\n');

  shell(`ipalpha_seed_local_clients '${root}'`);
  let clients = JSON.parse(readEnv(envFile('auth-api')).SEED_CLIENTS_JSON);
  for (const client of clients) {
    assert.deepEqual(Object.keys(client).sort(), ['clientId', 'scopes', 'secret', 'serviceId']);
    assert.ok(client.secret.length >= 8);
  }
  assert.ok(clients.find(c => c.serviceId === 'persons-api').scopes.includes('projects:read'));
  assert.ok(clients.find(c => c.serviceId === 'forms-api').scopes.includes('prefill:redeem'));
  const secrets = clients.map(c => c.secret);
  shell(`ipalpha_seed_local_clients '${root}'`);
  clients = JSON.parse(readEnv(envFile('auth-api')).SEED_CLIENTS_JSON);
  assert.deepEqual(clients.map(c => c.secret), secrets);

  // Migrate the legacy shape without discarding a separately configured client.
  const external = { clientId: 'other-internal', secret: 'operator-password', serviceId: 'other', scopes: ['apps:read'] };
  write(envFile('auth-api'), readEnv(envFile('auth-api')).MONGO_URI ? fs.readFileSync(envFile('auth-api'), 'utf8').replace(/^SEED_CLIENTS_JSON=.*$/m,
    `SEED_CLIENTS_JSON='${JSON.stringify([external, { clientId: 'persons-api', secret: 'old', ms: 'persons-api' }])}'`) : '');
  shell(`ipalpha_seed_local_clients '${root}'`);
  clients = JSON.parse(readEnv(envFile('auth-api')).SEED_CLIENTS_JSON);
  assert.deepEqual(clients.find(c => c.clientId === external.clientId), external);
  assert.equal(clients.find(c => c.clientId === 'persons-api').ms, undefined);

  // Rewrites are simultaneous, so chained port mappings do not collapse peers.
  write(envFile('projects-api'), 'PORT=3001\nAUTH_API_URL=http://localhost:3002\nPROJECTS_API_URL=http://localhost:3001\n');
  shell(`ipalpha_remap_from=(3001 3002); ipalpha_remap_to=(3002 3003); ipalpha_apply_port_rewrites '${root}'`);
  const projects = readEnv(envFile('projects-api'));
  assert.equal(projects.PORT, '3002');
  assert.equal(projects.AUTH_API_URL, 'http://localhost:3003');
  assert.equal(projects.PROJECTS_API_URL, 'http://localhost:3002');

  const ports = shell(`ipalpha_port_busy() { [[ "$1" == 3001 ]]; }; ipalpha_resolve_ports >/dev/null
for repo in "\${ipalpha_ms_order[@]}"; do ipalpha_settings_ms_port "$repo"; done`).trim().split('\n');
  assert.equal(new Set(ports).size, ports.length);
  assert.ok(Number(ports[0]) > 3009);
  console.log('local-env-test: all assertions passed');
} finally {
  fs.rmSync(root, { recursive: true, force: true });
}
