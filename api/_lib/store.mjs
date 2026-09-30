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

// Modes are deliberately NOT enumerated here. The client owns the list; the server only
// needs to know a name is safe as a blob path segment. That keeps adding a mode to one
// edit instead of two files that must be kept in agreement.
const MODE_PATTERN = /^[a-z][a-z0-9-]{0,23}$/;
export function isValidMode(mode) { return MODE_PATTERN.test(String(mode || '')); }

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
  // Derived from what is actually stored, so a mode the server has never heard of still
  // gets its URLs the moment its first blob is written.
  if (backend === 'fs') {
    const base = process.env.PM_LOCAL_BASE || '/__blob';
    let names = [];
    try { names = await fs.readdir(LOCAL_DIR); } catch { return {}; }
    return Object.fromEntries(names
      .map(name => [urlKeyFor(name), `${base}/${name}`])
      .filter(([key]) => key));
  }
  const { list } = await blobSdk();
  const { blobs } = await list({ token: process.env.BLOB_READ_WRITE_TOKEN });
  return Object.fromEntries(blobs
    .map(b => [urlKeyFor(b.pathname), b.url])
    .filter(([key]) => key));
}

/** `state-residential.json` -> `state:residential`; `kmz-industrial.kmz` -> `kmz:industrial`. */
function urlKeyFor(pathname) {
  if (pathname === VERSION_PATH) return 'version';
  const state = /^state-(.+)\.json$/.exec(pathname);
  if (state) return `state:${state[1]}`;
  const kmz = /^kmz-(.+)\.kmz$/.exec(pathname);
  if (kmz) return `kmz:${kmz[1]}`;
  return null;
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

export async function readVersions() {
  const current = await readJson(VERSION_PATH);
  // Absent modes simply have no entry; bumpVersion treats that as 0.
  if (!current) return { data: {}, etag: null };
  return { data: current.data || {}, etag: current.etag };
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
