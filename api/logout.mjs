import { clearCookie, send, isSecureEnv } from './_lib/auth.mjs';

export default async function handler(req, res) {
  if (req.method !== 'POST') return send(res, 405, { error: 'method not allowed' });
  if (req.headers['x-requested-with'] !== 'pm-admin') {
    return send(res, 403, { error: 'missing X-Requested-With' });
  }
  return send(res, 204, undefined, { 'set-cookie': clearCookie({ secure: isSecureEnv() }) });
}
