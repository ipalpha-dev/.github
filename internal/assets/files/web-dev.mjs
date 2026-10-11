// Starts a web app's own Vite config on the port ./run chose (IPALPHA_WEB_PORT). The tools pass
// every value through the environment: VITE_* (already moved to this run's ports, and Vite gives
// process.env priority over .env files) and <NAME>_API_URL for the app's /api proxy.
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
const port = Number(process.env.IPALPHA_WEB_PORT);
const server = await createServer({
  root,
  // node_modules can be shared across worktrees: never rewrite another workspace's optimizer cache.
  cacheDir: process.env.IPALPHA_VITE_CACHE || undefined,
  server: { port, strictPort: true },
  ...(process.argv.includes('--force') ? { optimizeDeps: { force: true } } : {}),
});
await server.listen();
server.printUrls();
