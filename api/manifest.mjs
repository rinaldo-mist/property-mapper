import { send } from './_lib/auth.mjs';
import { publicUrls, readVersions, backend, VERSION_MAX_AGE } from './_lib/store.mjs';

/**
 * Called once per page load. Hands the browser the absolute URLs it then reads directly
 * from the CDN, so the 30s polling loop costs no function invocations at all.
 */
export default async function handler(req, res) {
  if (req.method !== 'GET') return send(res, 405, { error: 'method not allowed' });
  try {
    const [urls, versions] = await Promise.all([publicUrls(), readVersions()]);
    return send(res, 200, {
      backend,
      urls,
      versions: versions.data,
      // The edge cache floor, so the client can size its poll interval honestly.
      propagationSeconds: VERSION_MAX_AGE
    }, { 'cache-control': 'no-store' });
  } catch (error) {
    console.error('manifest failed', error);
    return send(res, 503, { error: 'storage unavailable' });
  }
}
