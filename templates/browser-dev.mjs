import fs from 'node:fs';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { pathToFileURL } from 'node:url';

export const defaultBrowsers = ['auth-webapp', 'forms-webapp', 'mordomia-webapp', 'developers-webapp', 'mailpit'];
// Auth is a sign-in popup that other apps open; it still starts, but never gets its own tab.
export const neverAutoOpen = new Set(['auth-webapp']);
const exec = promisify(execFile);
const key = 'browser_apps';
const copy = {
  'pt-BR': {invalid: 'Página local desconhecida ou URL inválida.', failed: 'Não foi possível abrir o navegador.', timeout: 'Página local ainda indisponível.', saved: 'Preferências do navegador salvas.'},
  'en-US': {invalid: 'Unknown local page or invalid URL.', failed: 'Could not open the browser.', timeout: 'Local page is still unavailable.', saved: 'Browser preferences saved.'},
  es: {invalid: 'Página local desconocida o URL no válida.', failed: 'No se pudo abrir el navegador.', timeout: 'La página local aún no está disponible.', saved: 'Preferencias del navegador guardadas.'},
  fr: {invalid: 'Page locale inconnue ou URL non valide.', failed: 'Impossible d’ouvrir le navigateur.', timeout: 'La page locale est encore indisponible.', saved: 'Préférences du navigateur enregistrées.'},
  de: {invalid: 'Unbekannte lokale Seite oder ungültige URL.', failed: 'Der Browser konnte nicht geöffnet werden.', timeout: 'Die lokale Seite ist noch nicht verfügbar.', saved: 'Browser-Einstellungen gespeichert.'},
};

function settings(dir) {
  const file = path.join(dir, 'settings');
  const text = fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : '';
  const lang = /^lang=(.*)$/m.exec(text)?.[1] || 'pt-BR';
  return {file, text, messages: copy[lang] || copy['pt-BR']};
}

export function selectedBrowsers(dir) {
  const {text} = settings(dir);
  const match = /^browser_apps=(.*)$/m.exec(text);
  return [...new Set(match ? match[1].trim().split(/\s+/).filter(Boolean) : defaultBrowsers)];
}

export function saveBrowsers(dir, ids) {
  const {file, text} = settings(dir);
  if (ids.some(id => !/^[a-z0-9-]+$/.test(id))) throw new Error(settings(dir).messages.invalid);
  const line = `${key}=${[...new Set(ids)].join(' ')}`;
  const next = /^browser_apps=.*$/m.test(text)
    ? text.replace(/^browser_apps=.*$/m, () => line)
    : text.replace(/\n?$/, '\n') + line + '\n';
  const temporary = `${file}.browser-${randomUUID()}`;
  fs.mkdirSync(dir, {recursive: true});
  try {
    fs.writeFileSync(temporary, next, {mode: 0o600, flag: 'wx'});
    fs.renameSync(temporary, file);
  } finally {
    if (fs.existsSync(temporary)) fs.unlinkSync(temporary);
  }
}

export function browserPages(dir) {
  const cfg = JSON.parse(fs.readFileSync(path.join(dir, 'projects.json'), 'utf8'));
  return new Map(cfg.projects.filter(project => project.frontend).map(project => {
    const url = new URL(project.frontend);
    if (url.protocol !== 'http:' || !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) || url.username || url.password) {
      throw new Error(settings(dir).messages.invalid);
    }
    return [project.name, url.href];
  }));
}

export function selectApp(dir, id, enabled) {
  if (!browserPages(dir).has(id)) throw new Error(settings(dir).messages.invalid);
  const selected = selectedBrowsers(dir);
  saveBrowsers(dir, enabled ? [...selected, id] : selected.filter(name => name !== id));
  return enabled;
}

async function openBrowser(url) {
  const command = process.platform === 'darwin' ? 'open' : process.env.WSL_DISTRO_NAME ? 'wslview' : 'xdg-open';
  // argv only, never a shell. Keep a broken desktop opener from stalling the launcher.
  await exec(command, [url], {timeout: 5000});
}

export async function openReadyBrowsers(dir, {
  ids, timeout = 120_000, interval = 500, probeTimeout = 800,
  fetchImpl = fetch, open = openBrowser,
  wait = ms => new Promise(resolve => setTimeout(resolve, ms)),
  active = () => true,
} = {}) {
  const pages = browserPages(dir);
  const candidates = new Set(ids ?? selectedBrowsers(dir).filter(id => !neverAutoOpen.has(id)));
  const pending = new Set([...candidates].filter(id => pages.has(id)));
  const opened = [], failed = [];
  const deadline = Date.now() + timeout;
  while (pending.size && Date.now() < deadline && active()) {
    const selected = new Set(ids ?? selectedBrowsers(dir));
    await Promise.all([...pending].map(async id => {
      if (!selected.has(id)) { pending.delete(id); return; }
      const url = pages.get(id);
      let ready = false;
      try {
        const response = await fetchImpl(id === 'mailpit' ? new URL('/readyz', url).href : url, {
          signal: AbortSignal.timeout(probeTimeout), redirect: 'manual',
        });
        ready = response.status >= 200 && response.status < 400;
        await response.body?.cancel();
      } catch { /* Retry while the local process starts. */ }
      if (!ready || !active()) return;
      if (!ids && !selectedBrowsers(dir).includes(id)) { pending.delete(id); return; }
      pending.delete(id); // Never open a second tab for this page in the same run.
      try { await open(url); opened.push(id); }
      catch { failed.push(id); }
    }));
    if (pending.size) await wait(interval);
  }
  return {opened, failed, pending: [...pending]};
}

function parentAlive(pid) {
  try { process.kill(pid, 0); return true; } catch { return false; }
}

async function main([dir, command = 'list', ...ids]) {
  const {messages} = settings(dir);
  if (command === 'list') { console.log(JSON.stringify(selectedBrowsers(dir))); return; }
  if (command === 'enable' || command === 'disable') { console.log(selectApp(dir, ids[0], command === 'enable')); return; }
  if (command === 'set' || command === 'defaults') {
    const choices = command === 'defaults' ? defaultBrowsers : ids;
    const pages = browserPages(dir);
    if (choices.some(id => !pages.has(id) && !defaultBrowsers.includes(id))) throw new Error(messages.invalid);
    saveBrowsers(dir, choices);
    console.log(messages.saved);
    return;
  }
  if (command !== 'watch' && command !== 'open') throw new Error(messages.invalid);
  if (command === 'watch' && (process.env.IPALPHA_OPEN_BROWSERS === '0' || process.env.CI)) return;
  if (command === 'open' && (ids.length !== 1 || !browserPages(dir).has(ids[0]))) throw new Error(messages.invalid);
  if (!/^browser_apps=/m.test(settings(dir).text)) saveBrowsers(dir, selectedBrowsers(dir));
  const parent = process.ppid;
  const result = await openReadyBrowsers(dir, {
    ids: command === 'open' ? ids : undefined,
    active: () => parentAlive(parent),
  });
  if (command === 'open' && result.opened.length) {
    saveBrowsers(dir, [...selectedBrowsers(dir), ...result.opened]);
  }
  if (result.failed.length) throw new Error(messages.failed);
  if (command === 'open' && result.pending.length) throw new Error(messages.timeout);
}

if (process.argv[1] && import.meta.url === pathToFileURL(fs.realpathSync(process.argv[1])).href) {
  main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exitCode = 1; });
}
