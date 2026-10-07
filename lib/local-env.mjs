import fs from 'node:fs';
import path from 'node:path';
import { randomBytes, createHash } from 'node:crypto';
import os from 'node:os';
import { pathToFileURL } from 'node:url';

export function readEnv(file, allowHyphens = false) {
  if (!fs.existsSync(file)) return {};
  return Object.fromEntries(fs.readFileSync(file, 'utf8').split(/\r?\n/).flatMap(line => {
    const match = (allowHyphens ? /^([A-Za-z_][A-Za-z0-9_-]*)=(.*)$/ : /^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/).exec(line);
    if (!match) return [];
    let value = match[2].trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) value = value.slice(1, -1);
    return [[match[1], value]];
  }));
}

export function completeLocalEnv(repoDir, name, fallbackFile) {
  const file = path.join(repoDir, '.env');
  const root = path.resolve(repoDir, name === 'forms-api' ? '../../..' : '../..');
  const settings = readEnv(path.join(root, '.ipalpha/settings'), true);
  const auth = readEnv(path.join(root, 'core/auth-api/.env'));
  const values = readEnv(file);
  const defaults = readEnv(fallbackFile);
  const changes = {};
  const fill = (key, value) => {
    if (!values[key]?.trim() && value) values[key] = changes[key] = value;
  };
  const authPort = settings['auth-api_port'] || '3005';
  const issuer = auth.AUTH_TOKEN_ISSUER || `http://localhost:${authPort}`;
  for (const [key, value] of Object.entries(defaults)) fill(key, value);
  fill('SERVICE_NAME', name);
  // An explicitly empty INSTANCE_ID suppresses shared-js's hostname fallback.
  // Keep dev workspaces distinct when multiple copies run on the same machine.
  fill('INSTANCE_ID', `${os.hostname()}-${createHash('sha256').update(root).digest('hex').slice(0, 8)}`);
  fill('AUTH_TOKEN_ISSUER', issuer);
  // Existing examples used both localhost and 127.0.0.1 for iss. JWT issuer matching
  // is exact; converge only these known local defaults, never a custom/remote issuer.
  if (name !== 'auth-api' && new RegExp(`^http://(?:localhost|127\\.0\\.0\\.1):(?:3005|${authPort})$`).test(values.AUTH_TOKEN_ISSUER)) {
    values.AUTH_TOKEN_ISSUER = changes.AUTH_TOKEN_ISSUER = issuer;
  }
  fill('TOKEN_AUDIENCE', `ipalpha:${name.replace(/-api$/, '')}`);
  fill('AUTH_API_AUDIENCE', 'ipalpha:auth');
  for (const peer of ['projects', 'persons', 'organizations', 'notifications', 'forms', 'ai', 'developers', 'dispatch']) {
    fill(`${peer.toUpperCase()}_API_AUDIENCE`, `ipalpha:${peer}`);
  }
  // Correct the outdated local example ports (forms=3007, ai=3010), preserving
  // operator-defined hosts/ports. Workspace port remapping runs after completion.
  for (const [key, oldPort, newPort] of [['FORMS_API_URL', '3007', '3006'], ['AI_API_URL', '3010', '3008']]) {
    if (new RegExp(`^http://(?:localhost|127\\.0\\.0\\.1):${oldPort}/?$`).test(values[key])) {
      values[key] = changes[key] = `http://127.0.0.1:${newPort}`;
    }
  }
  if (/^redis:\/\/ipalpha:ipalpha@(localhost|127\.0\.0\.1):\d+\/?$/.test(values.REDIS_URL || '')) {
    // The local Redis container deliberately has no ACL/password configured.
    values.REDIS_URL = changes.REDIS_URL = values.REDIS_URL.replace('ipalpha:ipalpha@', '');
  }
  const ids = {
    FORMS_APP_ID: 'app-ipalpha-forms',
    FORMS_CREATOR_ENTRY_POINT_ID: 'ep-ipalpha-forms-creator',
    FORMS_RESPONDENT_ENTRY_POINT_ID: 'ep-ipalpha-forms-respondent',
    MORDOMIA_APP_ID: 'app-mordomia',
  }; // Auth-owned stable IDs (auth-api/src/builtin-apps.ts, amendment A1).
  if (['persons-api', 'forms-api', 'dispatch-api'].includes(name)) {
    for (const [key, value] of Object.entries(ids)) fill(key, value);
  }
  if (name === 'persons-api') fill('IMPORT_ROWS_KEY', randomBytes(32).toString('base64'));
  if (name === 'auth-api') fill('WEBHOOK_SECRET_KEY', randomBytes(32).toString('base64'));
  if (Object.keys(changes).length) {
    let text = fs.readFileSync(file, 'utf8');
    for (const [key, value] of Object.entries(changes)) {
      const line = `${key}=${value}`;
      const pattern = new RegExp(`^${key}=.*$`, 'm');
      text = pattern.test(text) ? text.replace(pattern, () => line) : text.replace(/\n?$/, '\n') + line + '\n';
    }
    const temporary = `${file}.setup-${process.pid}`;
    try {
      fs.writeFileSync(temporary, text, { mode: 0o600, flag: 'wx' });
      fs.renameSync(temporary, file);
    } finally {
      if (fs.existsSync(temporary)) fs.unlinkSync(temporary);
    }
  }
  fs.chmodSync(file, 0o600);
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  completeLocalEnv(...process.argv.slice(2));
}
