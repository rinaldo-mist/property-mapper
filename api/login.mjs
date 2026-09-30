import { verifyPassword, createSession, sessionCookie, send, readBody, isSecureEnv } from './_lib/auth.mjs';

// Raw body everywhere, so behaviour is identical under `vercel dev`, the local test
// server and production — a parser that has already drained the stream reads as empty.
export const config = { api: { bodyParser: false } };

// Best-effort only: serverless instances are ephemeral, so this slows a casual attacker
// rather than providing a real guarantee. The scrypt cost is the actual defence.
const attempts = new Map();
const WINDOW_MS = 60_000;
const MAX_ATTEMPTS = 8;

function tooManyAttempts(key) {
  const now = Date.now();
  const hits = (attempts.get(key) || []).filter(t => now - t < WINDOW_MS);
  hits.push(now);
  attempts.set(key, hits);
  return hits.length > MAX_ATTEMPTS;
}

export default async function handler(req, res) {
  if (req.method !== 'POST') return send(res, 405, { error: 'method not allowed' });
  if (req.headers['x-requested-with'] !== 'pm-admin') {
    return send(res, 403, { error: 'missing X-Requested-With' });
  }
  const secret = process.env.SESSION_SECRET;
  const stored = process.env.ADMIN_PASSWORD_HASH;
  if (!secret || !stored) {
    return send(res, 500, { error: 'server is missing SESSION_SECRET or ADMIN_PASSWORD_HASH' });
  }
  const ip = (req.headers['x-forwarded-for'] || '').split(',')[0].trim() || 'local';
  if (tooManyAttempts(ip)) return send(res, 429, { error: 'too many attempts' });

  let password = '';
  try {
    password = JSON.parse((await readBody(req, 4096)).toString('utf8') || '{}').password || '';
  } catch { /* falls through to the generic failure below */ }

  if (!verifyPassword(password, stored)) {
    // Deliberately generic: nothing here should hint at why it failed.
    return send(res, 401, { error: 'invalid credentials' });
  }
  return send(res, 204, undefined, {
    'set-cookie': sessionCookie(createSession(secret), { secure: isSecureEnv() })
  });
}
