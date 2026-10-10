import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { normalizePhone, normalizeName, writeSuperuser } from '../lib/superuser.mjs';
import { readEnv } from '../lib/local-env.mjs';

const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-superuser-test-'));
try {
  for (const phone of ['99900000000', '(99) 90000-0000', '+55 99 90000-0000', '099900000000']) {
    assert.equal(normalizePhone(phone), '+5599900000000');
  }
  for (const bad of ['', '900000000', '00900000000', '9980000000', '+1 99900000000']) {
    assert.throws(() => normalizePhone(bad), /invalidPhone/);
  }
  assert.equal(normalizeName('  Joao  Silva Costa  '), 'Joao Silva Costa');
  assert.throws(() => normalizeName('Name\nSUPERUSER_PHONE=other'), /invalidName/);
  assert.throws(() => normalizeName('x'.repeat(201)), /invalidName/);
  const file = path.join(fixture, 'core/auth-api/.env');
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, 'SUPERUSER_NAME=\nSUPERUSER_PHONE=\nAUTH_CLIENT_SECRET=keep-secret\n');
  const name = "Joao $USER O'Neal";
  writeSuperuser(file, name, '99900000000');
  assert.equal(readEnv(file).SUPERUSER_NAME, name);
  assert.equal(readEnv(file).SUPERUSER_PHONE, '+5599900000000');
  assert.equal(readEnv(file).AUTH_CLIENT_SECRET, 'keep-secret');
  if (process.platform !== 'win32') {
    const link = path.join(fixture, 'linked-helper.mjs');
    fs.symlinkSync(path.join(tooling, 'lib/superuser.mjs'), link);
    const normalized = execFileSync(process.execPath, [link, 'normalize', 'phone'], { input: '99900000000', encoding: 'utf8' });
    assert.equal(normalized, '+5599900000000', 'CLI entry point must also run through a symlink');
  }
  if (process.platform !== 'win32') assert.equal(fs.statSync(file).mode & 0o777, 0o600);
  const before = fs.readFileSync(file, 'utf8');
  assert.throws(() => writeSuperuser(file, 'Name', '123'), /invalidPhone/);
  assert.equal(fs.readFileSync(file, 'utf8'), before, 'bad input must not partially replace seed configuration');
  const env = { ...process.env, IPALPHA_SUPERUSER_NAME: '', IPALPHA_SUPERUSER_PHONE: '' };
  const script = `set -euo pipefail
source lib/common.sh
source lib/i18n.sh
source lib/env.sh
source lib/superuser.sh
ipalpha_prompt_superuser "$TEST_ROOT"
ipalpha_write_superuser "$TEST_ROOT"`;
  execFileSync(process.env.IPALPHA_BASH || '/bin/bash', ['-c', script], { cwd: tooling, env: { ...env, TEST_ROOT: fixture }, stdio: 'pipe' });
  assert.equal(readEnv(file).SUPERUSER_NAME, name, 'repeat setup keeps the existing identity');
  const missing = path.join(fixture, 'new-workspace');
  assert.throws(() => execFileSync(process.env.IPALPHA_BASH || '/bin/bash', ['-c', script], { cwd: tooling, env: { ...env, TEST_ROOT: missing }, stdio: 'pipe' }), error => error.status === 1);
  for (const lang of ['pt-BR', 'en-US', 'es', 'fr', 'de']) {
    const text = execFileSync(process.env.IPALPHA_BASH || '/bin/bash', ['-c', `source lib/i18n.sh; ipalpha_lang='${lang}'; ipalpha_msg seed_phone_invalid`], { cwd: tooling, encoding: 'utf8' });
    assert.notEqual(text.trim(), 'seed_phone_invalid');
  }
  console.log('superuser-test: all assertions passed');
} finally { fs.rmSync(fixture, { recursive: true, force: true }); }
