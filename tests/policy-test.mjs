import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const checker = path.join(root, 'scripts/policy-check.mjs');
const policy = path.join(root, 'policy');
const run = (...args) => spawnSync(process.execPath, [checker, '--policy', policy, ...args], { encoding: 'utf8' });

let result = run();
assert.equal(result.status, 0, result.stderr);
assert.match(result.stdout, /policy check passed/);

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-policy-'));
try {
  fs.mkdirSync(path.join(tmp, '.ipalpha'), { recursive: true });
  fs.cpSync(policy, path.join(tmp, '.ipalpha/policy'), { recursive: true });
  fs.copyFileSync(path.join(root, 'templates/workspace-AGENTS.md'), path.join(tmp, 'AGENTS.md'));
  const repo = path.join(tmp, 'core/example-api');
  fs.mkdirSync(repo, { recursive: true });
  execFileSync('git', ['init', '-q', repo]);
  fs.writeFileSync(path.join(repo, '.env.example'), 'VALUE=\n');
  execFileSync('git', ['-C', repo, 'add', '.env.example']);

  result = run('--workspace', tmp);
  assert.equal(result.status, 0, result.stderr);

  fs.writeFileSync(path.join(repo, '.env'), 'SYNTHETIC_TEST_VALUE=fixture\n');
  execFileSync('git', ['-C', repo, 'add', '-f', '.env']);
  result = run('--workspace', tmp);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /tracks forbidden environment file/);
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}

console.log('policy-test: all assertions passed');
