import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import http from 'node:http';
import {spawn, execFile, execFileSync} from 'node:child_process';
import {promisify} from 'node:util';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {defaultBrowsers, neverAutoOpen, selectedBrowsers, saveBrowsers, selectApp, browserPages, openReadyBrowsers} from '../templates/browser-dev.mjs';

const tooling = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'ipalpha-browser-test-'));
const root = path.join(fixture, 'Workspace with spaces');
const dir = path.join(root, '.ipalpha');
const write = (file, text, mode = 0o600) => { fs.mkdirSync(path.dirname(file), {recursive: true}); fs.writeFileSync(file, text, {mode}); };
const servers = [];
let runner;
let runnerExit;
let runnerOutput = '';
const stopRunner = async () => {
  if (!runner) return;
  try { process.kill(-runner.pid, 'SIGTERM'); } catch {}
  await runnerExit;
  runner = undefined;
};
const shell = script => execFileSync('/bin/bash', ['-eu', '-c', `
  for lib in common i18n settings generate; do source "$1/lib/$lib.sh"; done
  ${script}
`, 'browser-test', tooling, root], {encoding: 'utf8'});

try {
  write(path.join(dir, 'settings'), 'lang=en-US\nCUSTOM_VALUE=preserved\n');
  assert.deepEqual(selectedBrowsers(dir), defaultBrowsers);
  saveBrowsers(dir, []);
  assert.deepEqual(selectedBrowsers(dir), []);

  shell('ipalpha_write_settings "$2"');
  assert.deepEqual(selectedBrowsers(dir), [], 'setup must preserve an empty selection');
  saveBrowsers(dir, ['forms-webapp', 'mailpit']);
  shell('ipalpha_write_settings "$2"');
  assert.deepEqual(selectedBrowsers(dir), ['forms-webapp', 'mailpit']);
  fs.appendFileSync(path.join(dir, 'settings'), 'CUSTOM_VALUE=preserved\n');

  const projects = [];
  for (const name of [...defaultBrowsers, 'forms-webapp', 'extra-webapp']) {
    let attempts = 0;
    const server = http.createServer((req, res) => {
      attempts += 1;
      // Exercise retry and redirects without ever opening a real desktop browser.
      res.statusCode = attempts === 1 ? 503 : name === 'auth-webapp' ? 302 : 200;
      res.end('local test page');
    });
    server.listen(0, '127.0.0.1');
    await once(server, 'listening');
    servers.push(server);
    projects.push({name, kind: name === 'mailpit' ? 'browser' : 'app', frontend: `http://127.0.0.1:${server.address().port}/`});
  }
  write(path.join(dir, 'projects.json'), JSON.stringify({projects}));
  assert.equal(selectApp(dir, 'forms-webapp', false), false);
  assert.equal(selectApp(dir, 'auth-webapp', true), true);
  assert.deepEqual(selectedBrowsers(dir), ['mailpit', 'auth-webapp']);
  assert.match(fs.readFileSync(path.join(dir, 'settings'), 'utf8'), /CUSTOM_VALUE=preserved/);
  assert.equal(fs.statSync(path.join(dir, 'settings')).mode & 0o777, 0o600);
  assert.throws(() => selectApp(dir, 'unknown', true));
  assert.throws(() => saveBrowsers(dir, ['bad\nKEY=value']));

  saveBrowsers(dir, defaultBrowsers);
  const opened = [];
  const result = await openReadyBrowsers(dir, {timeout: 1000, interval: 5, open: async url => opened.push(url)});
  const autoOpened = defaultBrowsers.filter(id => !neverAutoOpen.has(id));
  assert.deepEqual(new Set(result.opened), new Set(autoOpened));
  assert.equal(opened.length, autoOpened.length);
  assert.equal(new Set(opened).size, autoOpened.length, 'only one tab per selected page');
  assert.ok(!opened.includes(projects[0].frontend), 'auth-webapp is never opened automatically');
  assert.deepEqual(result.pending, []);
  for (const page of projects.slice(defaultBrowsers.length)) assert.ok(!opened.includes(page.frontend));

  // An unavailable page does not open, a failed opener is not retried, and cancellation stops work.
  let opens = 0;
  let failed = await openReadyBrowsers(dir, {ids: ['mailpit'], timeout: 25, interval: 1,
    fetchImpl: async () => new Response('', {status: 503}), open: async () => { opens += 1; }});
  assert.deepEqual(failed.pending, ['mailpit']);
  assert.equal(opens, 0);
  failed = await openReadyBrowsers(dir, {ids: ['mailpit'], fetchImpl: async () => new Response(''),
    open: async () => { opens += 1; throw new Error('unavailable desktop'); }});
  assert.deepEqual(failed.failed, ['mailpit']);
  assert.equal(opens, 1);
  const cancelled = await openReadyBrowsers(dir, {active: () => false, open: async () => assert.fail('cancelled launcher')});
  assert.equal(cancelled.opened.length, 0);
  saveBrowsers(dir, ['mailpit']);
  await openReadyBrowsers(dir, {interval: 0,
    fetchImpl: async () => { saveBrowsers(dir, []); return new Response('', {status: 503}); },
    open: async () => assert.fail('disabled while waiting'),
  });
  assert.deepEqual(selectedBrowsers(dir), []);
  saveBrowsers(dir, ['mailpit']);
  await openReadyBrowsers(dir, {
    fetchImpl: async () => { saveBrowsers(dir, []); return new Response(''); },
    open: async () => assert.fail('disabled during the readiness request'),
  });
  assert.deepEqual(selectedBrowsers(dir), []);

  const goodManifest = fs.readFileSync(path.join(dir, 'projects.json'), 'utf8');
  for (const url of ['https://example.com/', 'file:///etc/passwd', 'http://user:password@localhost:5100/']) {
    write(path.join(dir, 'projects.json'), JSON.stringify({projects: [{name: 'unsafe', frontend: url}]}));
    assert.throws(() => browserPages(dir));
  }
  write(path.join(dir, 'projects.json'), goodManifest);

  // The generated ./run opens the remembered set again in a fresh process, across runners.
  const bin = path.join(dir, 'bin');
  fs.mkdirSync(bin, {recursive: true});
  fs.mkdirSync(path.join(dir, '.state'));
  for (const lib of ['common', 'i18n', 'settings', 'env', 'ports']) {
    fs.mkdirSync(path.join(dir, 'lib'), {recursive: true});
    fs.copyFileSync(path.join(tooling, `lib/${lib}.sh`), path.join(dir, `lib/${lib}.sh`));
  }
  fs.copyFileSync(path.join(tooling, 'templates/browser-dev.mjs'), path.join(bin, 'browser-dev.mjs'));
  for (const name of ['infra-up', 'infra-down', 'install-deps', 'auth-keys-bootstrap']) write(path.join(bin, name), '#!/bin/sh\nexit 0\n', 0o700);
  write(path.join(bin, 'fallback-run'), `#!${process.execPath}\nsetInterval(() => {}, 1000);\n`, 0o700);
  shell('ipalpha_write_root_run "$2"');
  const openLog = path.join(fixture, 'opened.jsonl');
  const fakeBin = path.join(fixture, 'fake-bin');
  for (const name of ['open', 'xdg-open', 'wslview']) {
    write(path.join(fakeBin, name), `#!${process.execPath}\nrequire('fs').appendFileSync(process.env.BROWSER_TEST_LOG, JSON.stringify(process.argv[2]) + '\\n');\n`, 0o700);
  }
  const launchAndWait = async count => {
    runnerOutput = '';
    runner = spawn(path.join(root, 'run'), [], {detached: true, env: {...process.env,
      CI: '', IPALPHA_RUNNER: 'background', IPALPHA_OPEN_BROWSERS: '1',
      PATH: `${fakeBin}:${process.env.PATH}`, BROWSER_TEST_LOG: openLog,
    }, stdio: ['ignore', 'pipe', 'pipe']});
    runnerExit = once(runner, 'exit');
    runner.stdout.on('data', chunk => { runnerOutput += chunk; });
    runner.stderr.on('data', chunk => { runnerOutput += chunk; });
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      const urls = fs.existsSync(openLog) ? fs.readFileSync(openLog, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
      if (urls.length === count) { await stopRunner(); return urls; }
      if (runner.exitCode !== null) throw new Error(runnerOutput);
      await new Promise(resolve => setTimeout(resolve, 20));
    }
    const browserLog = path.join(dir, '.state/browser.log');
    throw new Error(`browser launcher timed out: ${runnerOutput}\n${fs.existsSync(browserLog) ? fs.readFileSync(browserLog, 'utf8') : 'no browser log'}\n${fs.existsSync(openLog) ? fs.readFileSync(openLog, 'utf8') : 'no opener log'}`);
  };
  saveBrowsers(dir, defaultBrowsers);
  const first = await launchAndWait(autoOpened.length);
  assert.deepEqual(new Set(first), new Set(projects.filter(p => autoOpened.includes(p.name)).map(p => p.frontend)));
  for (const [command, id] of [['enable', 'extra-webapp'], ['disable', 'mordomia-webapp'], ['disable', 'auth-webapp']]) {
    execFileSync(process.execPath, [path.join(bin, 'browser-dev.mjs'), dir, command, id]);
  }
  const remembered = ['mailpit', 'extra-webapp'];
  const second = await launchAndWait(autoOpened.length + remembered.length);
  assert.deepEqual(new Set(second.slice(autoOpened.length)), new Set(projects.filter(p => remembered.includes(p.name)).map(p => p.frontend)));
  assert.deepEqual(selectedBrowsers(dir), remembered);
  const listed = JSON.parse(execFileSync(path.join(root, 'run'), ['browsers'], {encoding: 'utf8'}));
  assert.deepEqual(listed, remembered);
  execFileSync(path.join(root, 'run'), ['browsers', 'set']);
  assert.deepEqual(selectedBrowsers(dir), []);
  execFileSync(path.join(root, 'run'), ['browsers', 'defaults']);
  assert.deepEqual(selectedBrowsers(dir), defaultBrowsers);
  const desktopEnv = {...process.env, PATH: `${fakeBin}:${process.env.PATH}`, BROWSER_TEST_LOG: openLog};
  const previousLog = fs.readFileSync(openLog, 'utf8');
  for (const overrides of [{CI: '1', IPALPHA_OPEN_BROWSERS: '1'}, {CI: '', IPALPHA_OPEN_BROWSERS: '0'}]) {
    execFileSync(process.execPath, [path.join(bin, 'browser-dev.mjs'), dir, 'watch'], {env: {...desktopEnv, ...overrides}, timeout: 2000});
    assert.equal(fs.readFileSync(openLog, 'utf8'), previousLog);
    assert.deepEqual(selectedBrowsers(dir), defaultBrowsers);
  }
  await promisify(execFile)(process.execPath, [path.join(bin, 'browser-dev.mjs'), dir, 'open', 'extra-webapp'], {env: desktopEnv, timeout: 2000});
  assert.ok(selectedBrowsers(dir).includes('extra-webapp'), 'manual opening must be remembered');
  assert.ok(fs.readFileSync(openLog, 'utf8').includes(projects.find(p => p.name === 'extra-webapp').frontend));

  write(path.join(root, 'core/auth-webapp/package.json'), '{}');
  write(path.join(root, 'core/mordomia-webapp/package.json'), '{}');
  shell(`ipalpha_port_mailpit=18025; ipalpha_port_auth_webapp=15100; ipalpha_port_mordomia_webapp=15110
    ipalpha_write_projects_json "$2" "$2/.ipalpha/generated-projects.json"`);
  const generated = JSON.parse(fs.readFileSync(path.join(dir, 'generated-projects.json'), 'utf8')).projects;
  assert.equal(generated.find(p => p.name === 'mailpit').frontend, 'http://127.0.0.1:18025/');
  assert.equal(generated.find(p => p.name === 'mailpit').kind, 'browser');
  assert.equal(generated.find(p => p.name === 'mailpit').autostart, false);
  assert.equal(generated.find(p => p.name === 'auth-webapp').frontend, 'http://localhost:15100/');
  assert.equal(generated.find(p => p.name === 'mordomia-webapp').frontend, 'http://localhost:15110/');
  assert.equal(generated.find(p => p.name === 'auth-webapp').autostart, true);
  assert.equal(generated.find(p => p.name === 'mordomia-webapp').autostart, true);
  // The headless runner and mprocs start exactly the saved web apps, without changing APIs.
  write(path.join(root, 'apps/forms/forms-webapp/package.json'), '{}');
  const startLog = path.join(fixture, 'started.jsonl');
  write(path.join(bin, 'web-dev'), `#!${process.execPath}\nrequire('fs').appendFileSync(${JSON.stringify(startLog)}, JSON.stringify(process.argv[2]) + '\\n'); setInterval(() => {}, 1000);\n`, 0o700);
  shell('ipalpha_write_bin_fallback_run "$2/.ipalpha/bin/fallback-run"');
  const saved = ['auth-webapp', 'forms-webapp'];
  saveBrowsers(dir, saved);
  runner = spawn(path.join(bin, 'fallback-run'), [], {detached: true,
    env: {...process.env, TMPDIR: `${fixture}/`}, stdio: ['ignore', 'pipe', 'pipe']});
  runnerExit = once(runner, 'exit');
  const deadline = Date.now() + 3000;
  let started = [];
  while (Date.now() < deadline) {
    started = fs.existsSync(startLog) ? fs.readFileSync(startLog, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
    if (started.length === 2) break;
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  assert.deepEqual(new Set(started), new Set(saved));
  await stopRunner();
  assert.deepEqual(selectedBrowsers(dir), saved, 'whole-run shutdown preserves saved apps');
  shell('ipalpha_load_settings "$2"; ipalpha_write_mprocs_yaml "$2" "$2/.ipalpha/mprocs.yaml"');
  const yaml = fs.readFileSync(path.join(dir, 'mprocs.yaml'), 'utf8');
  for (const name of saved) assert.match(yaml, new RegExp(`Web · ${name}[^]*?autostart: true`));
  assert.match(yaml, /Web · mordomia-webapp[^]*?autostart: false/);
  const selectedNow = JSON.parse(execFileSync(path.join(root, 'run'), ['apps'], {encoding: 'utf8'}));
  assert.deepEqual(selectedNow, saved);
  assert.ok(!fs.readdirSync(dir).some(name => name.includes('.browser-')));
  console.log('browser-test: all assertions passed (real local HTTP, mocked desktop opener)');
} finally {
  await stopRunner();
  await Promise.all(servers.map(server => new Promise(resolve => server.close(resolve))));
  fs.rmSync(fixture, {recursive: true, force: true});
}
