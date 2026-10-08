import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { readEnv } from './local-env.mjs';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);

// Dev-only: a reinstalled workspace can reuse a Mongo volume seeded with older secrets,
// and auth-api seeds clients insert-if-absent. Re-hash only the drifted local system clients.
export async function reconcileClients(collection, argon2, clients) {
  const updated = [];
  for (const { clientId, secret } of clients) {
    const doc = await collection.findOne({ clientId, clientClass: 'system' });
    if (!doc) continue;
    if (doc.secretHash && await argon2.verify(doc.secretHash, secret).catch(() => false)) continue;
    await collection.updateOne(
      { _id: doc._id },
      { $set: { secretHash: await argon2.hash(secret), rotatedAt: new Date() }, $inc: { credentialVersion: 1 } },
    );
    updated.push(clientId);
  }
  return updated;
}

export async function syncLocalClients(root) {
  const authDir = path.join(root, 'core/auth-api');
  const env = readEnv(path.join(authDir, '.env'));
  if (!env.MONGO_URI || !env.SEED_CLIENTS_JSON || !fs.existsSync(path.join(authDir, 'node_modules'))) return [];
  if (!LOCAL_HOSTS.has(new URL(env.MONGO_URI).hostname)) return [];
  const load = createRequire(path.join(authDir, 'package.json'));
  const argon2 = load('argon2');
  const { MongoClient } = load('mongoose').mongo;
  const client = new MongoClient(env.MONGO_URI, { serverSelectionTimeoutMS: 5000 });
  await client.connect();
  try {
    return await reconcileClients(client.db().collection('clients'), argon2, JSON.parse(env.SEED_CLIENTS_JSON));
  } finally {
    await client.close();
  }
}

if (process.argv[1] && fs.existsSync(process.argv[1]) && import.meta.url === pathToFileURL(fs.realpathSync(process.argv[1])).href) {
  syncLocalClients(process.argv[2])
    .then(updated => { if (updated.length) console.log(updated.join(' ')); })
    .catch(error => { console.error(error.message); process.exitCode = 1; });
}
