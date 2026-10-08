import assert from 'node:assert/strict';
import { reconcileClients } from '../lib/local-clients.mjs';

const argon2 = {
  hash: async secret => `hash:${secret}`,
  verify: async (hash, secret) => hash === `hash:${secret}`,
};
const docs = [
  { _id: 1, clientId: 'persons-api', clientClass: 'system', secretHash: 'hash:old', credentialVersion: 1 },
  { _id: 2, clientId: 'projects-api', clientClass: 'system', secretHash: 'hash:same', credentialVersion: 3 },
  { _id: 3, clientId: 'third-party', clientClass: 'confidential', secretHash: 'hash:x', credentialVersion: 1 },
];
const collection = {
  findOne: async query => docs.find(d => d.clientId === query.clientId && d.clientClass === query.clientClass) ?? null,
  updateOne: async ({ _id }, { $set, $inc }) => {
    const doc = docs.find(d => d._id === _id);
    Object.assign(doc, $set);
    doc.credentialVersion += $inc.credentialVersion;
  },
};

const updated = await reconcileClients(collection, argon2, [
  { clientId: 'persons-api', secret: 'new' },
  { clientId: 'projects-api', secret: 'same' },
  { clientId: 'missing-api', secret: 'x' },
  { clientId: 'third-party', secret: 'y' },
]);
assert.deepEqual(updated, ['persons-api'], 'only drifted system clients are re-hashed');
assert.equal(docs[0].secretHash, 'hash:new');
assert.equal(docs[0].credentialVersion, 2);
assert.ok(docs[0].rotatedAt instanceof Date);
assert.equal(docs[1].credentialVersion, 3, 'matching secrets are left alone');
assert.equal(docs[2].secretHash, 'hash:x', 'non-system clients are never touched');
assert.equal(docs.length, 3, 'missing clients are left for auth-api to seed');
assert.deepEqual(await reconcileClients(collection, argon2, [{ clientId: 'persons-api', secret: 'new' }]), [], 'idempotent');
console.log('local-clients-test: all assertions passed');
