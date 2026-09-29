import path from 'node:path';
import { pathToFileURL } from 'node:url';

export default async (env) => {
  const root = process.env.IPALPHA_FRONTEND_ROOT;
  const vite = await import(pathToFileURL(path.join(root, 'node_modules/vite/dist/node/index.js')).href);
  const loaded = await vite.loadConfigFromFile(env, undefined, root);
  const own = loaded?.config ?? {};
  const prefix = (own.base ?? '/frontend/').replace(/\/+$/, '');
  const trailingSlash = {
    name: 'ipalpha-trailing-slash',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const [pathname, query] = (req.url ?? '').split('?');
        if (pathname !== prefix) return next();
        res.statusCode = 301;
        res.setHeader('Location', `${prefix}/${query ? `?${query}` : ''}`);
        res.end();
      });
    },
  };
  return vite.mergeConfig(own, {
    root,
    plugins: [trailingSlash],
    configFile: false,
    server: {
      port: Number(process.env.IPALPHA_WEB_PORT),
      strictPort: true,
      proxy: {
        [`^(?!${prefix}(/|$)).*`]: {
          target: `http://127.0.0.1:${process.env.IPALPHA_API_PORT}`,
          changeOrigin: true,
          ws: true,
        },
      },
    },
  });
};
