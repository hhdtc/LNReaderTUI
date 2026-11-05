/**
 * Novel reader plugin, host half.
 *
 * Mounts the panel's HTTP surface on the DSH Web listener. Reading and library
 * writes are answered in-process from the LNReaderTUI data directory, so the
 * panel and the terminal reader share one library and one reading position;
 * site work is delegated to the `lnreadertui` CLI.
 */

import { API_PREFIX, createNovelReader } from './routes.js';

/** Cordis function-plugin name. */
export const name = 'novel-reader';

/** The route carrier and the connection trust fence every request passes. */
export const inject = ['webServer', 'connection'];

/**
 * Register the panel's routes for the life of this plugin.
 * @param {import('@deepseek-ai/cordis').Context} ctx
 * @param {{dataDir?: string, bin?: string}} [config] - optional profile configuration.
 */
export function apply(ctx, config = {}) {
  const server = createNovelReader({
    dataDir: typeof config?.dataDir === 'string' ? config.dataDir : undefined,
    bin: typeof config?.bin === 'string' ? config.bin : undefined,
  });
  ctx.effect(() => () => server.dispose(), 'novel-reader: jobs, caches, and readers');
  ctx.effect(() => ctx.webServer.register({
    kind: 'prefix',
    path: API_PREFIX,
    handler: async (request, response) => {
      if (rejectUntrusted(ctx, request, response)) return;
      await server.handle(request, response);
    },
  }), `novel-reader: ${API_PREFIX}`);
}

/**
 * Apply the same trust fence the built-in routes use: the connection service
 * rejects requests from authorities this Host does not serve.
 * @param {any} ctx
 * @param {import('node:http').IncomingMessage} request
 * @param {import('node:http').ServerResponse} response
 * @returns {boolean} true when the request was rejected.
 */
function rejectUntrusted(ctx, request, response) {
  const connection = Reflect.get(ctx, 'connection');
  if (connection === undefined || connection === null || typeof connection.requestRejection !== 'function') {
    return false;
  }
  const rejection = connection.requestRejection(request);
  if (rejection === undefined) return false;
  response.statusCode = rejection;
  response.setHeader('cache-control', 'no-store');
  response.end();
  return true;
}
