import { isAdmin, send } from './_lib/auth.mjs';

export default async function handler(req, res) {
  if (req.method !== 'GET') return send(res, 405, { error: 'method not allowed' });
  // Never cached: the answer is per-session and drives whether edit controls appear.
  return send(res, 200, { admin: isAdmin(req) }, { 'cache-control': 'no-store' });
}
