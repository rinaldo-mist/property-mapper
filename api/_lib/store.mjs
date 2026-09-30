// The only module that knows where data lives. Two adapters share one interface:
//
//   blob — Vercel Blob, used in production. Object storage behind a CDN, so viewers can
//          read version/state/KMZ straight from the edge with no function invoked and
//          no database to wake. That is why this is Blob and not Postgres: the polling
//          loop would otherwise keep a Neon instance awake for as long as any tab is open.
//   fs   — a plain directory, selected by PM_LOCAL_STORE. Used by `vercel dev` and by
//          the test suite, so the whole API can be exercised without any cloud account.
import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';

export const MODES = ['residential', 'industrial'];
export const VERSION_PATH = 'version.json';
export const statePath = mode => `state-${mode}.json`;
export const kmzPath = mode => `kmz-${mode}.kmz`;

// Vercel Blob enforces a 60s floor on cacheControlMaxAge, so an edge-cached version file
// can lag by up to a minute. That is the real propagation bound for viewers.
export const VERSION_MAX_AGE = 60;
export const DATA_MAX_AGE = 60;

const LOCAL_DIR = process.env.PM_LOCAL_STORE || '';
export const backend = LOCAL_DIR ? 'fs' : 'blob';

function localFile(pathname) { return path.join(LOCAL_DIR, pathname.replace(/[\\/]+/g, '_')); }
function etagOf(buffer) { return `"${crypto.createHash('sha256').update(buffer).digest('hex').slice(0, 32)}"`; }

async function blobSdk() {
  try {
    return await import('@vercel/blob');
  } catch {
    throw new Error('@vercel/blob is not installed. Run: npm install');
  }
}

/** Absolute public URLs the browser reads directly. */
export async function publicUrls() {
  if (backend === 'fs') {
    const base = process.env.PM_LOCAL_BASE || '/__blob';
    // Only paths that actually exist, matching what list() reports for Blob. Advertising
    // a URL for a blob that was never written made the client fetch a 404 instead of
    // falling back to the bundled KMZ — and only locally, which is the worst kind of bug.
    const present = async pathname =>
      fs.access(localFile(pathname)).then(() => `${base}/${pathname}`).catch(() => null);
    const entries = await Promise.all([
      present(VERSION_PATH).then(url => ['version', url]),
      ...MODES.flatMap(m => [
        present(statePath(m)).then(url => [`state:${m}`, url]),
        present(kmzPath(m)).then(url => [`kmz:${m}`, url])
      ])
    ]);
    return Object.fromEntries(entries);
  }
  const { list } = await blobSdk();
  const { blobs } = await list({ token: process.env.BLOB_READ_WRITE_TOKEN });
  const byPath = Object.fromEntries(blobs.map(b => [b.pathname, b.url]));
  const entry = p => byPath[p] || null;
  return Object.fromEntries([
    ['version', entry(VERSION_PATH)],
    ...MODES.flatMap(m => [[`state:${m}`, entry(statePath(m))], [`kmz:${m}`, entry(kmzPath(m))]])
  ]);
}

export async function readJson(pathname) {
  if (backend === 'fs') {
    try {
      const buffer = await fs.readFile(localFile(pathname));
      return { data: JSON.parse(buffer.toString('utf8')), etag: etagOf(buffer) };
    } catch { return null; }
  }
  const { head } = await blobSdk();
  let meta;
  try {
    meta = await head(pathname, { token: process.env.BLOB_READ_WRITE_TOKEN });
  } catch { return null; }
  // The blob is public, so a plain fetch reads it; cache-busted so a write is seen
  // immediately by the server even while the edge still serves the previous copy.
  const response = await fetch(`${meta.url}?_=${Date.now()}`, { cache: 'no-store' });
  if (!response.ok) return null;
  return { data: await response.json(), etag: meta.etag };
}

export async function writeJson(pathname, data, { maxAge = DATA_MAX_AGE, ifMatch } = {}) {
  const body = Buffer.from(JSON.stringify(data), 'utf8');
  if (backend === 'fs') {
    await fs.mkdir(LOCAL_DIR, { recursive: true });
    if (ifMatch) {
      const current = await readJson(pathname);
      if (current && current.etag !== ifMatch) { const e = new Error('etag mismatch'); e.code = 'PRECONDITION'; throw e; }
    }
    await fs.writeFile(localFile(pathname), body);
    return { etag: etagOf(body) };
  }
  const { put } = await blobSdk();
  const result = await put(pathname, body, {
    access: 'public',
    contentType: 'application/json',
    // Defaults to false — without it every write after the first fails.
    allowOverwrite: true,
    addRandomSuffix: false,
    cacheControlMaxAge: maxAge,
    ...(ifMatch ? { ifMatch } : {}),
    token: process.env.BLOB_READ_WRITE_TOKEN
  });
  return { etag: result.etag, url: result.url };
}

export async function writeBytes(pathname, buffer, contentType, { maxAge = DATA_MAX_AGE } = {}) {
  if (backend === 'fs') {
    await fs.mkdir(LOCAL_DIR, { recursive: true });
    await fs.writeFile(localFile(pathname), buffer);
    return { etag: etagOf(buffer) };
  }
  const { put } = await blobSdk();
  const result = await put(pathname, buffer, {
    access: 'public',
    contentType,
    allowOverwrite: true,
    addRandomSuffix: false,
    cacheControlMaxAge: maxAge,
    token: process.env.BLOB_READ_WRITE_TOKEN
  });
  return { etag: result.etag, url: result.url };
}

function emptyVersions() { return Object.fromEntries(MODES.map(m => [m, 0])); }

export async function readVersions() {
  const current = await readJson(VERSION_PATH);
  if (!current) return { data: emptyVersions(), etag: null };
  return { data: { ...emptyVersions(), ...current.data }, etag: current.etag };
}

/**
 * Bump one mode's version. Uses the ETag as a compare-and-set so two admins saving at
 * the same instant cannot lose one another's bump; a clash simply retries against the
 * value that won.
 */
export async function bumpVersion(mode, attempts = 3) {
  for (let i = 0; i < attempts; i++) {
    const { data, etag } = await readVersions();
    const next = { ...data, [mode]: (data[mode] || 0) + 1, updatedAt: new Date().toISOString() };
    try {
      await writeJson(VERSION_PATH, next, { maxAge: VERSION_MAX_AGE, ifMatch: etag || undefined });
      return next;
    } catch (error) {
      const conflict = error && (error.code === 'PRECONDITION' || /412|precondition|etag/i.test(error.message || ''));
      if (!conflict || i === attempts - 1) throw error;
    }
  }
  throw new Error('could not bump version');
}
