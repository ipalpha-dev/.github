// Opt-in integrated test: real checked-out APIs, separate containers, no personal data.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { readEnv } from '../lib/local-env.mjs';

const exec = promisify(execFile);
const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = process.argv[2];
assert.ok(source, 'usage: node tests/fresh-start-test.mjs /path/to/ipalpha');
const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-cold-start-'));
const root = path.join(fixture, 'IpAlpha');
const infra = `ipalpha-qa-${process.pid}`;
const apis = ['projects-api', 'persons-api', 'organizations-api', 'notifications-api', 'auth-api', 'forms-api', 'ai-api', 'developers-api', 'dispatch-api'];
const repoPath = (workspace, name) => path.join(workspace, name.startsWith('forms-') ? 'apps/forms' : 'core', name);
const environment = { ...process.env, IPALPHA_TEST_WORKSPACE: source,
  IPALPHA_CLONE_COMMAND: path.join(tooling, 'tests/workspace-clone'),
  IPALPHA_TARGET_DIR: root, IPALPHA_LANG: 'en-US', IPALPHA_SKIP_INSTALL: '1', IPALPHA_NO_SHELL: '1',
  IPALPHA_SUPERUSER_PHONE: '99900000000',
  IPALPHA_SUPERUSER_NAME: 'Joao Silva Costa',
  ipalpha_infra_name: infra, IPALPHA_RUNNER: 'background',
  // Existing dependency installs are reused; all source, dist, envs, caches and DBs are fresh.
  IPALPHA_PROCS_BINARY: path.join(source, '.ipalpha/bin/ipalpha-procs'),
};
let runner;
let runnerOutput = '';
const run = async (command, args, options = {}) => {
  try { return await exec(command, args, { env: environment, maxBuffer: 20 * 1024 * 1024, ...options }); }
  catch (error) { throw new Error(`${command} ${args.join(' ')} failed: ${error.stderr || error.stdout || error.message}`); }
};
try {
  await run('/bin/bash', [path.join(tooling, 'setup'), '--skip-tools', '--keep-setup']);
  const settings = readEnv(path.join(root, '.ipalpha/settings'), true);
  assert.equal(settings.infra_name, infra);
  assert.equal(new Set(apis.map(api => settings[`${api}_port`])).size, apis.length);
  // This headless seed check must not open the developer's desktop browser tabs.
  const settingsFile = path.join(root, '.ipalpha/settings');
  const settingsText = fs.readFileSync(settingsFile, 'utf8');
  fs.writeFileSync(settingsFile, /^browser_apps=.*$/m.test(settingsText)
    ? settingsText.replace(/^browser_apps=.*$/m, 'browser_apps=') : settingsText + '\nbrowser_apps=\n');
  for (const name of ['shared-js', 'shared-ui', ...apis, 'auth-webapp', 'forms-webapp', 'mordomia-webapp', 'developers-webapp']) {
    const deps = path.join(repoPath(source, name), 'node_modules');
    assert.ok(fs.existsSync(deps), `${name} needs an installed source dependency tree`);
    fs.symlinkSync(deps, path.join(repoPath(root, name), 'node_modules'), 'dir');
  }
  runner = spawn('./run', [], { cwd: root, env: { ...environment, TMPDIR: fixture + '/' }, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
  runner.stdout.on('data', chunk => { runnerOutput += chunk; });
  runner.stderr.on('data', chunk => { runnerOutput += chunk; });
  runner.on('error', error => { runnerOutput += error.message; });
  const deadline = Date.now() + 180_000;
  const pending = new Set(apis);
  while (pending.size && Date.now() < deadline) {
    for (const api of [...pending]) {
      try {
        const response = await fetch(`http://127.0.0.1:${settings[`${api}_port`]}/ready`, { signal: AbortSignal.timeout(800) });
        if (response.ok && (await response.json()).ready) {
          pending.delete(api);
          console.log(`PASS: fresh ${api} /ready`);
        }
      } catch {}
    }
    if (pending.size) await new Promise(resolve => setTimeout(resolve, 500));
  }
  if (pending.size) {
    const logs = path.join(fixture, 'ipalpha-run-logs');
    let errors = '';
    for (const api of pending) {
      const file = path.join(logs, `${api}.log`);
      if (fs.existsSync(file)) errors += `${api}: ${fs.readFileSync(file, 'utf8').slice(-2000)}\n`;
    }
    throw new Error(`APIs not ready: ${[...pending].join(', ')}\n${runnerOutput}\n${errors}`);
  }
  const require = createRequire(path.join(repoPath(source, 'auth-api'), 'package.json'));
  const { createConnection } = require('mongoose');
  const authEnv = readEnv(path.join(repoPath(root, 'auth-api'), '.env'));
  const connection = await createConnection(authEnv.MONGO_URI, { dbName: 'auth' }).asPromise();
  try {
    const before = await connection.db.collection('keyRings').findOne({ _id: 'auth' });
    assert.ok(before, 'the generated ./run must have bootstrapped auth keys');
    await run(path.join(root, '.ipalpha/bin/auth-keys-bootstrap'), []);
    const after = await connection.db.collection('keyRings').findOne({ _id: before._id });
    assert.equal(after.currentSigningKid, before.currentSigningKid);
    assert.equal(after.currentEncryptionKid, before.currentEncryptionKid);
    console.log('PASS: repeat key bootstrap preserves existing keys');
    const personsEnv = readEnv(path.join(repoPath(root, 'persons-api'), '.env'));
    const personsConnection = await createConnection(personsEnv.MONGO_URI, { dbName: 'persons' }).asPromise();
    try {
      let person;
      let superuser;
      const seedDeadline = Date.now() + 60_000;
      while (Date.now() < seedDeadline) {
        person = await personsConnection.db.collection('people').findOne({ 'phones.e164': '+5599900000000' });
        if (person) superuser = await connection.db.collection('superusers').findOne({ personId: String(person._id) });
        if (superuser) break;
        await new Promise(resolve => setTimeout(resolve, 500));
      }
      if (!superuser) {
        const log = fs.readFileSync(path.join(fixture, 'ipalpha-run-logs/auth-api.log'), 'utf8');
        const seedLogs = log.split('\n').filter(line => /superuser seed|person create failed|person lookup failed/.test(line));
        throw new Error(`Initial superuser seed did not finish: ${seedLogs.slice(-6).join('\n')}`);
      }
      assert.equal(person.name, 'Joao Silva Costa');
      assert.equal(await personsConnection.db.collection('people').countDocuments({ 'phones.e164': '+5599900000000' }), 1);
      console.log('PASS: initial name/phone create one canonical person and grant the central superuser role');
    } finally { await personsConnection.close(); }
  } finally { await connection.close(); }
} finally {
  if (runner?.pid) {
    try { process.kill(-runner.pid, 'SIGTERM'); } catch {}
    await new Promise(resolve => setTimeout(resolve, 300));
  }
  const runtimeFile = path.join(root, '.ipalpha/.state/runtime');
  if (fs.existsSync(runtimeFile) && fs.readFileSync(runtimeFile, 'utf8').trim() === 'docker') {
    await exec('docker', ['compose', '--env-file', path.join(root, '.ipalpha/.env'),
      '--env-file', path.join(root, '.ipalpha/ports.env'), '-f', path.join(root, '.ipalpha/compose.yaml'),
      'down', '--volumes']).catch(() => {});
  }
  // Names are unique to this test; never stop or remove shared development infra.
  for (const kind of ['mongo', 'redis', 'rabbitmq', 'mailpit']) {
    const name = `${infra}-${kind}`;
    await exec('container', ['stop', name]).catch(() => {});
    await exec('container', ['delete', name]).catch(() => {});
    await exec('container', ['volume', 'delete', `${name}-data`]).catch(() => {});
  }
  await exec('container', ['network', 'delete', infra]).catch(() => {});
  fs.rmSync(fixture, { recursive: true, force: true });
}
