import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { completeLocalEnv, readEnv } from '../lib/local-env.mjs';

const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-mailpit-test-'));
const repo = path.join(root, 'core/notifications-api');
const file = path.join(repo, '.env');
const fallback = path.join(tooling, 'templates/env-fallback/notifications-api.env');
const bash = process.env.IPALPHA_BASH || '/bin/bash';
const shell = script => execFileSync(bash, ['-eu', '-c', `
  for lib in common i18n env ports settings generate; do source "$1/lib/$lib.sh"; done
  ${script}
`, 'mailpit-test', tooling, root], { encoding: 'utf8' });

try {
  fs.mkdirSync(repo, { recursive: true });
  fs.mkdirSync(path.join(root, '.ipalpha'));
  fs.copyFileSync(fallback, file);
  completeLocalEnv(repo, 'notifications-api', fallback);
  const env = readEnv(file);
  assert.equal(env.MAIL_PROVIDER, 'mailpit');
  assert.equal(env.SMS_PROVIDER, 'mailpit');
  assert.equal(env.DEPLOYMENT_ENVIRONMENT, 'development');
  assert.equal(env.MAILPIT_USERNAME, '');
  assert.equal(env.MAILPIT_PASSWORD, '');

  // A busy inbox port is remapped consistently in settings, infra and the API.
  shell(`
    ipalpha_i18n_init en-US
    ipalpha_port_busy() { [[ "$1" == 8025 ]]; }
    ipalpha_resolve_ports >/dev/null
    ipalpha_apply_port_rewrites "$2"
    ipalpha_write_settings "$2"
    ipalpha_write_ports_env "$2/.ipalpha/ports.env"
    unset ipalpha_port_mailpit
    ipalpha_load_settings "$2"
    [[ "$ipalpha_port_mailpit" == 8026 ]]
    ipalpha_rewrites_from_settings
    ipalpha_apply_port_rewrites "$2"
  `);
  assert.equal(readEnv(file).MAILPIT_URL, 'http://127.0.0.1:8026');
  assert.equal(readEnv(path.join(root, '.ipalpha/ports.env')).MAILPIT_HOST_PORT, '8026');
  assert.equal(readEnv(path.join(root, '.ipalpha/settings')).mailpit_port, '8026');

  // The repo's .env.example names the paid providers with empty keys: local sends go to Mailpit.
  fs.writeFileSync(file, 'MAIL_PROVIDER=sendgrid\nSENDGRID_API_KEY=\nSMS_PROVIDER=smsbarato\nSMSBARATO_KEY=\nDEPLOYMENT_ENVIRONMENT=production\n');
  completeLocalEnv(repo, 'notifications-api', fallback);
  const example = readEnv(file);
  assert.equal(example.MAIL_PROVIDER, 'mailpit');
  assert.equal(example.SMS_PROVIDER, 'mailpit');
  assert.equal(example.DEPLOYMENT_ENVIRONMENT, 'development');

  // Local setup must never enable paid delivery, even when credentials are present.
  fs.writeFileSync(file, 'MAIL_PROVIDER=sendgrid\nSMS_PROVIDER=smsbarato\nSENDGRID_API_KEY=operator-value\nSMSBARATO_KEY=operator-sms\nMAILPIT_URL=http://127.0.0.1:8999\n');
  completeLocalEnv(repo, 'notifications-api', fallback);
  const custom = readEnv(file);
  assert.equal(custom.MAIL_PROVIDER, 'mailpit');
  assert.equal(custom.SMS_PROVIDER, 'mailpit');
  assert.equal(custom.DEPLOYMENT_ENVIRONMENT, 'development');
  assert.equal(custom.SENDGRID_API_KEY, 'operator-value');
  assert.equal(custom.SMSBARATO_KEY, 'operator-sms');
  assert.equal(custom.MAILPIT_URL, 'http://127.0.0.1:8999');
  const before = fs.readFileSync(file, 'utf8');
  completeLocalEnv(repo, 'notifications-api', fallback);
  assert.equal(fs.readFileSync(file, 'utf8'), before, 'Mailpit enforcement must be idempotent');

  for (const name of ['infra-up', 'infra-down', 'infra-logs']) {
    shell(`ipalpha_write_bin_${name.replaceAll('-', '_')} "$2/${name}"`);
    const script = fs.readFileSync(path.join(root, name), 'utf8');
    assert.ok(script.includes('$infra-mailpit'), `${name} must include Mailpit`);
    execFileSync(bash, ['-n', path.join(root, name)]);
  }
  const compose = fs.readFileSync(path.join(tooling, 'templates/compose.yaml'), 'utf8');
  assert.ok(compose.includes('127.0.0.1:${MAILPIT_HOST_PORT:-8025}:8025'));
  assert.ok(!compose.includes(':1025'), 'SMTP must not be published');
  console.log('mailpit-test: all assertions passed');
} finally {
  fs.rmSync(root, { recursive: true, force: true });
}
