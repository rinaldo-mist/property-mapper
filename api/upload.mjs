import { guardMutation, send, readBody } from './_lib/auth.mjs';
import { writeBytes, bumpVersion, kmzPath, MODES, DATA_MAX_AGE } from './_lib/store.mjs';

export const config = { api: { bodyParser: false } };

const KMZ_TYPE = 'application/vnd.google-earth.kmz';
const MAX_BYTES = 25 * 1024 * 1024;

/**
 * Raw bytes, not multipart. Two reasons: no parser dependency, and a KMZ content type is
 * not CORS-simple, so a cross-site <form> cannot reach this route at all.
 */
export default async function handler(req, res) {
  if (req.method !== 'POST') return send(res, 405, { error: 'method not allowed' });
  if (!guardMutation(req, res)) return;

  const mode = new URL(req.url, 'http://localhost').searchParams.get('mode');
  if (!MODES.includes(mode)) return send(res, 400, { error: 'unknown mode' });

  let body;
  try {
    body = await readBody(req, MAX_BYTES);
  } catch {
    return send(res, 413, { error: 'file too large' });
  }
  if (!body.length) return send(res, 400, { error: 'empty body' });
  // A KMZ is a zip: reject anything that is not, before it reaches storage.
  if (!(body[0] === 0x50 && body[1] === 0x4b)) {
    return send(res, 415, { error: 'not a KMZ/KML archive' });
  }
  try {
    await writeBytes(kmzPath(mode), body, KMZ_TYPE, { maxAge: DATA_MAX_AGE });
    const versions = await bumpVersion(mode);
    return send(res, 200, { version: versions[mode], versions, bytes: body.length });
  } catch (error) {
    console.error('upload failed', error);
    return send(res, 503, { error: 'storage unavailable' });
  }
}
