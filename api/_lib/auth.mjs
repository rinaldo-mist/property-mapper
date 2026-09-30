// Session and password handling. No dependencies: node:crypto covers scrypt and HMAC.
//
// The point of this module is that the admin check happens on a server. A value shipped
// to the browser — hardcoded, or inlined from an env var at build time — is readable by
// anyone who opens devtools, so it can never be a security boundary.
import crypto from 'node:crypto';

export const COOKIE_NAME = 'pm_session';
export const SESSION_TTL_MS = 12 * 60 * 60 * 1000;
const SCRYPT = { N: 16384, r: 8, p: 1 };
const KEY_LEN = 64;

/** Generate the value for ADMIN_PASSWORD_HASH. See the CLI at the bottom of this file. */
export function hashPassword(password, salt = crypto.randomBytes(16)) {
  const derived = crypto.scryptSync(String(password), salt, KEY_LEN, SCRYPT);
  return `scrypt$${salt.toString('hex')}$${derived.toString('hex')}`;
}

export function verifyPassword(password, stored) {
  const parts = String(stored || '').split('$');
  if (parts.length !== 3 || parts[0] !== 'scrypt') return false;
  const [, saltHex, hashHex] = parts;
  let expected;
  try { expected = Buffer.from(hashHex, 'hex'); } catch { return false; }
  if (!expected.length) return false;
  let actual;
  try {
    actual = crypto.scryptSync(String(password), Buffer.from(saltHex, 'hex'), expected.length, SCRYPT);
  } catch { return false; }
  // Lengths match by construction; timingSafeEqual throws if they ever do not.
  return actual.length === expected.length && crypto.timingSafeEqual(actual, expected);
}

function sign(body, secret) {
  return crypto.createHmac('sha256', secret).update(body).digest('base64url');
}

export function createSession(secret, now = Date.now()) {
  const body = Buffer.from(JSON.stringify({ exp: now + SESSION_TTL_MS })).toString('base64url');
  return `${body}.${sign(body, secret)}`;
}

/** The payload for a valid, unexpired session, else null. */
export function readSession(token, secret, now = Date.now()) {
  if (!token || !secret) return null;
  const [body, mac] = String(token).split('.');
  if (!body || !mac) return null;
  const a = Buffer.from(mac);
  const b = Buffer.from(sign(body, secret));
  if (a.length !== b.length || !crypto.timingSafeEqual(a, b)) return null;
  try {
    const payload = JSON.parse(Buffer.from(body, 'base64url').toString('utf8'));
    return typeof payload.exp === 'number' && payload.exp > now ? payload : null;
  } catch { return null; }
}

export function parseCookies(header) {
  const out = {};
  String(header || '').split(';').forEach(part => {
    const i = part.indexOf('=');
    if (i < 0) return;
    out[part.slice(0, i).trim()] = decodeURIComponent(part.slice(i + 1).trim());
  });
  return out;
}

export function sessionCookie(value, { secure = true, maxAge = SESSION_TTL_MS / 1000 } = {}) {
  // HttpOnly so script cannot read it; Lax so a cross-site form POST carries no session.
  return [
    `${COOKIE_NAME}=${value}`,
    'HttpOnly', 'SameSite=Lax', 'Path=/',
    secure ? 'Secure' : '',
    `Max-Age=${maxAge}`
  ].filter(Boolean).join('; ');
}

export function clearCookie(options = {}) {
  return sessionCookie('', { ...options, maxAge: 0 });
}

export function isAdmin(req) {
  const token = parseCookies(req.headers && req.headers.cookie)[COOKIE_NAME];
  return !!readSession(token, process.env.SESSION_SECRET);
}

/**
 * Guards a mutating route. Returns true when the request may proceed.
 *
 * The X-Requested-With check is belt-and-braces: uploads arrive as a raw KMZ content
 * type, which is NOT a CORS-simple type, so a cross-site <form> cannot send one in the
 * first place. A custom header additionally forces a preflight on anything that could.
 */
export function guardMutation(req, res) {
  if (req.headers['x-requested-with'] !== 'pm-admin') {
    send(res, 403, { error: 'missing X-Requested-With' });
    return false;
  }
  if (req.headers['sec-fetch-site'] === 'cross-site') {
    send(res, 403, { error: 'cross-site request rejected' });
    return false;
  }
  if (!isAdmin(req)) {
    send(res, 401, { error: 'admin session required' });
    return false;
  }
  return true;
}

export function send(res, status, body, headers = {}) {
  res.statusCode = status;
  Object.entries(headers).forEach(([k, v]) => res.setHeader(k, v));
  if (body === undefined) return res.end();
  res.setHeader('content-type', 'application/json; charset=utf-8');
  res.end(JSON.stringify(body));
}

export async function readBody(req, limitBytes = 25 * 1024 * 1024) {
  const chunks = [];
  let total = 0;
  for await (const chunk of req) {
    total += chunk.length;
    if (total > limitBytes) throw new Error('payload too large');
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

export function isSecureEnv() {
  return process.env.PM_INSECURE_COOKIES !== '1';
}

// node api/_lib/auth.mjs hash "my password"
if (process.argv[1] && process.argv[1].replace(/\\/g, '/').endsWith('api/_lib/auth.mjs') && process.argv[2] === 'hash') {
  const password = process.argv[3];
  if (!password) {
    console.error('usage: node api/_lib/auth.mjs hash "<password>"');
    process.exit(1);
  }
  console.log(hashPassword(password));
}
