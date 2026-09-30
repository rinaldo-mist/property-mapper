import { guardMutation, send, readBody } from './_lib/auth.mjs';
import { writeJson, bumpVersion, statePath, MODES, DATA_MAX_AGE } from './_lib/store.mjs';

export const config = { api: { bodyParser: false } };

/** Admin-only. Viewers read the state blob straight from the CDN via /api/manifest. */
export default async function handler(req, res) {
  if (req.method !== 'POST') return send(res, 405, { error: 'method not allowed' });
  if (!guardMutation(req, res)) return;

  const mode = new URL(req.url, 'http://localhost').searchParams.get('mode');
  if (!MODES.includes(mode)) return send(res, 400, { error: 'unknown mode' });

  let payload;
  try {
    payload = JSON.parse((await readBody(req, 8 * 1024 * 1024)).toString('utf8'));
  } catch {
    return send(res, 400, { error: 'body must be JSON' });
  }
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return send(res, 400, { error: 'body must be a JSON object' });
  }
  try {
    await writeJson(statePath(mode), payload, { maxAge: DATA_MAX_AGE });
    const versions = await bumpVersion(mode);
    return send(res, 200, { version: versions[mode], versions });
  } catch (error) {
    console.error('state write failed', error);
    return send(res, 503, { error: 'storage unavailable' });
  }
}
