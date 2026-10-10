import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-clone-test-'));
const worker = path.join(fixture, 'worker.cjs');
fs.writeFileSync(worker, `#!/usr/bin/env node
const fs = require('fs'), path = require('path');
const repo = path.basename(process.argv[2], '.git'), dest = process.argv[3], state = process.env.CLONE_TEST_STATE;
const running = path.join(state, repo + '.running');
fs.writeFileSync(running, String(process.pid));
fs.writeFileSync(path.join(state, repo + '.started'), JSON.stringify({pid: process.pid, active: fs.readdirSync(state).filter(f => f.endsWith('.running')).length}));
process.on('SIGTERM', () => { fs.rmSync(running, {force:true}); process.exit(143); });
setTimeout(() => {
  fs.rmSync(running, {force:true});
  if (repo === process.env.CLONE_TEST_FAIL) process.exit(1);
  fs.mkdirSync(path.join(dest, '.git'), {recursive:true});
}, Number(process.env.CLONE_TEST_DELAY || 150));
`, { mode: 0o755 });
const script = `set -euo pipefail
source lib/common.sh
source lib/i18n.sh
source lib/clone.sh
ipalpha_all_repos() { printf '%s\\n' alpha beta gamma optional delta; }
ipalpha_is_app_repo() { [[ "$1" == optional ]]; }
trap 'exit 130' INT
trap 'exit 143' TERM
before="$(trap -p INT)"
status=0
ipalpha_clone_org_repos "$CLONE_TEST_ROOT" || status=$?
[[ "$(trap -p INT)" == "$before" ]]
exit "$status"
`;
let activeProcess;
const children = new Set();
const prepare = (name, extra = {}) => {
  const state = path.join(fixture, name, 'state');
  fs.mkdirSync(state, { recursive: true });
  return { cwd: tooling, env: { ...process.env, CLONE_TEST_STATE: state,
    CLONE_TEST_ROOT: path.join(fixture, name, 'workspace with spaces'),
    IPALPHA_CLONE_COMMAND: worker, IPALPHA_CLONE_JOBS: '2', TMPDIR: fixture + '/', ...extra } };
};
try {
  const bash = process.env.IPALPHA_BASH || '/bin/bash';
  const parallel = prepare('parallel');
  execFileSync(bash, ['-c', script], parallel);
  const records = fs.readdirSync(parallel.env.CLONE_TEST_STATE).filter(f => f.endsWith('.started'))
    .map(f => JSON.parse(fs.readFileSync(path.join(parallel.env.CLONE_TEST_STATE, f), 'utf8')));
  assert.equal(records.length, 5);
  assert.ok(records.some(r => r.active === 2), 'clones must overlap');
  assert.ok(records.every(r => r.active <= 2), 'the worker limit must be respected');
  execFileSync(bash, ['-c', script], parallel); // keeps existing checkouts
  assert.equal(fs.readdirSync(parallel.env.CLONE_TEST_STATE).filter(f => f.endsWith('.started')).length, 5);
  execFileSync(bash, ['-c', script], prepare('optional', { CLONE_TEST_FAIL: 'optional' }));
  assert.throws(() => execFileSync(bash, ['-c', script], prepare('required', { CLONE_TEST_FAIL: 'beta' })), error => error.status === 1);
  assert.throws(() => execFileSync(bash, ['-c', script], prepare('invalid', { IPALPHA_CLONE_JOBS: '0' })), error => error.status === 1);

  // Native Windows does not preserve POSIX process-group/signal semantics; WSL/Linux covers cancellation.
  if (process.platform !== 'win32') {
    const cancellation = prepare('cancel', { CLONE_TEST_DELAY: '30000' });
    activeProcess = spawn(bash, ['-c', script], { ...cancellation, stdio: 'ignore' });
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline && fs.readdirSync(cancellation.env.CLONE_TEST_STATE).filter(f => f.endsWith('.started')).length < 2) {
      await new Promise(resolve => setTimeout(resolve, 20));
    }
    const started = fs.readdirSync(cancellation.env.CLONE_TEST_STATE).filter(f => f.endsWith('.started'));
    assert.equal(started.length, 2);
    for (const name of started) children.add(JSON.parse(fs.readFileSync(path.join(cancellation.env.CLONE_TEST_STATE, name), 'utf8')).pid);
    const exited = new Promise(resolve => activeProcess.once('exit', resolve));
    activeProcess.kill('SIGTERM');
    assert.equal(await Promise.race([exited, new Promise(resolve => setTimeout(() => resolve('timeout'), 5000))]), 143);
    for (const pid of children) assert.throws(() => process.kill(pid, 0), error => error.code === 'ESRCH');
    children.clear();
    assert.equal(fs.readdirSync(fixture).filter(f => f.startsWith('ipalpha-clone.')).length, 0, 'temporary pool state must be removed');
  }
  console.log('clone-test: all assertions passed');
} finally {
  if (activeProcess?.exitCode === null) activeProcess.kill('SIGTERM');
  for (const pid of children) { try { process.kill(pid, 'SIGTERM'); } catch {} }
  fs.rmSync(fixture, { recursive: true, force: true });
}
