import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { readEnv } from '../lib/local-env.mjs';

const root = process.cwd();
const require = createRequire(path.join(root, 'package.json'));
const vitePackage = require.resolve('vite/package.json');
const { createServer, loadEnv } = await import(pathToFileURL(path.join(path.dirname(vitePackage), 'dist/node/index.js')).href);
const pkg = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
const workspace = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const auth = readEnv(path.join(workspace, 'core/auth-api/.env'));
const settings = readEnv(path.join(workspace, '.ipalpha/settings'), true);
const port = Number(process.argv[2]);
const authWebappUrl = `http://localhost:${settings['auth-webapp_port'] || 5100}`;
const defaults = { VITE_AUTH_WEBAPP_URL: authWebappUrl };
if (pkg.name === 'oikos-webapp') Object.assign(defaults, {
  VITE_APP_CLIENT_ID: auth.BUILTIN_OIKOS_CLIENT_ID || 'oikos-webapp',
  VITE_ENTRY_POINT_KEY: 'church',
  VITE_AUTH_CALLBACK_URI: `http://localhost:${port}/auth/callback`,
  VITE_FORMS_URL: `http://localhost:${settings['forms-webapp_port'] || 5106}`,
});
if (pkg.name === 'developers-webapp') Object.assign(defaults, {
  VITE_APP_CLIENT_ID: auth.BUILTIN_DEVELOPERS_CLIENT_ID || 'developers-web',
  VITE_ENTRY_POINT_KEY: 'portal',
  VITE_AUTH_CALLBACK_URI: `http://localhost:${port}/auth/callback`,
});
if (pkg.name === 'forms-webapp') Object.assign(defaults, {
  VITE_CREATOR_CLIENT_ID: auth.BUILTIN_FORMS_CREATOR_CLIENT_ID || 'forms-creator-web',
  VITE_RESPONDENT_CLIENT_ID: auth.BUILTIN_FORMS_RESPONDENT_CLIENT_ID || 'forms-respondent-web',
  VITE_CREATOR_ENTRY_POINT: 'creator', VITE_RESPONDENT_ENTRY_POINT: 'respondent',
  VITE_CREATOR_CALLBACK_URI: `http://localhost:${port}/auth/callback`,
  VITE_RESPONDENT_CALLBACK_URI: `http://localhost:${port}/respond/callback`,
});
const existing = loadEnv('development', root, 'VITE_');
for (const [key, value] of Object.entries(defaults)) {
  if (!process.env[key]?.trim() && !existing[key]?.trim()) process.env[key] = value;
}
const include = [];
if (pkg.dependencies?.['@ipalpha/shared-ui']) {
  // Excluded/linked ESM libraries still need their CommonJS transitive imports
  // prebundled. Vite's nested dependency syntax keeps shared-ui itself excluded.
  for (const dependency of ['@tiptap/react', '@tiptap/starter-kit', '@tiptap/extensions']) {
    include.push(`@ipalpha/shared-ui > ${dependency}`);
  }
}
const server = await createServer({
  root,
  // node_modules can be linked across worktrees. Never rewrite another running
  // workspace's optimizer cache when this workspace starts or uses --force.
  cacheDir: path.join(workspace, '.ipalpha/.vite', pkg.name),
  server: { port, strictPort: true },
  optimizeDeps: { include, ...(process.argv.includes('--force') ? { force: true } : {}) },
});
await server.listen();
server.printUrls();
server.bindCLIShortcuts({ print: true });
