// Starts a web app's own Vite config on the port ./run chose (IPALPHA_WEB_PORT). The tools pass
// every value through the environment: VITE_* (already moved to this run's ports, and Vite gives
// process.env priority over .env files) and <NAME>_API_URL for the app's /api proxy.
import path from 'node:path';
import { readFileSync } from 'node:fs';
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
const cert = process.env.IPALPHA_WEB_TLS_CERT;
const key = process.env.IPALPHA_WEB_TLS_KEY;
const origin = process.env.IPALPHA_WEB_ORIGIN;
if (Boolean(cert) !== Boolean(key) || (cert && !origin)) throw new Error('Local HTTPS requires IPALPHA_WEB_TLS_CERT, IPALPHA_WEB_TLS_KEY and IPALPHA_WEB_ORIGIN together.');
const publicUrl = origin ? new URL(origin) : null;
if (publicUrl && (publicUrl.origin !== origin || (cert ? publicUrl.protocol !== 'https:' : publicUrl.protocol !== 'http:') || Number(publicUrl.port || (cert ? 443 : 80)) !== port)) {
  throw new Error('IPALPHA_WEB_ORIGIN must be a bare origin with the protocol and actual port selected by ./run.');
}
const server = await createServer({
  root,
  // node_modules can be shared across worktrees: never rewrite another workspace's optimizer cache.
  cacheDir: process.env.IPALPHA_VITE_CACHE || undefined,
  server: {
    port, strictPort: true,
    ...(publicUrl ? { host: '127.0.0.1', origin: publicUrl.origin, allowedHosts: [publicUrl.hostname] } : {}),
    ...(cert && key ? { https: { cert: readFileSync(cert), key: readFileSync(key) } } : {}),
  },
  ...(process.argv.includes('--force') ? { optimizeDeps: { force: true } } : {}),
});
await server.listen();
server.printUrls();
