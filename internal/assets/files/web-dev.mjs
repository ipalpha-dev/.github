// Starts a web app's own Vite config on the port ./run chose (IPALPHA_WEB_PORT). The tools pass
// every value through the environment: VITE_* (already moved to this run's ports, and Vite gives
// process.env priority over .env files) and <NAME>_API_URL for the app's /api proxy.
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const root = process.cwd();
const require = createRequire(path.join(root, 'package.json'));
let vitePackage;
try {
  vitePackage = require.resolve('vite/package.json');
} catch {
  console.error(`vite is not installed in ${root} — run ./run again (it installs dependencies) or npm install there.`);
  process.exit(1);
}
const { createServer } = await import(pathToFileURL(path.join(path.dirname(vitePackage), 'dist/node/index.js')).href);
const pkg = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
const port = Number(process.env.IPALPHA_WEB_PORT);
const include = [];
if (pkg.dependencies?.['@ipalpha/shared-ui']) {
  // Excluded/linked ESM libraries still need their CommonJS transitive imports prebundled.
  for (const dependency of ['@tiptap/react', '@tiptap/starter-kit', '@tiptap/extensions']) {
    include.push(`@ipalpha/shared-ui > ${dependency}`);
  }
}
const server = await createServer({
  root,
  // node_modules can be shared across worktrees: never rewrite another workspace's optimizer cache.
  cacheDir: process.env.IPALPHA_VITE_CACHE || undefined,
  server: { port, strictPort: true },
  optimizeDeps: { include, ...(process.argv.includes('--force') ? { force: true } : {}) },
});
await server.listen();
server.printUrls();
