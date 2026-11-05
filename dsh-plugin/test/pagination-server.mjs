/**
 * Serves the pagination harness: the real client bundle, React 18 UMD builds,
 * and a fake `/novel-reader/api`. Used by `test/pagination.mjs` — and by hand
 * when checking the reader in a browser:
 *
 *   node test/pagination-server.mjs 4799
 *   # → http://127.0.0.1:4799/pagination.html
 *
 * The harness page copies its measurements into `#report` as JSON.
 */

import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join, normalize } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const pluginRoot = join(here, '..');
const devModules = join(pluginRoot, '.reacttest', 'node_modules');

const files = {
  '/': join(here, 'pagination.html'),
  '/pagination.html': join(here, 'pagination.html'),
  '/client.js': join(pluginRoot, 'lib', 'client.js'),
  '/react.js': join(devModules, 'react', 'umd', 'react.development.js'),
  '/react-dom.js': join(devModules, 'react-dom', 'umd', 'react-dom.development.js'),
};

const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
};

/**
 * Start the harness server.
 * @param {number} [port] - 0 picks a free port.
 * @param {(report: any) => void} [onReport] - called when the page reports.
 * @returns {Promise<{url: string, port: number, close: () => Promise<void>, lastReport: () => any}>}
 */
export function startServer(port = 0, onReport) {
  let report = null;
  const server = createServer(async (request, response) => {
    const path = normalize(new URL(String(request.url), 'http://localhost').pathname);
    if (path === '/log') {
      let body = '';
      for await (const chunk of request) body += chunk;
      if (body !== '') console.log(`LOG ${body}`);
      response.statusCode = 204;
      response.end();
      return;
    }
    if (path === '/report') {
      if (request.method === 'POST') {
        let body = '';
        for await (const chunk of request) body += chunk;
        try {
          report = JSON.parse(body);
        } catch {
          report = { ok: false, steps: [{ step: 'fail', message: 'malformed report' }] };
        }
        onReport?.(report);
        response.statusCode = 204;
        response.end();
        return;
      }
      response.setHeader('content-type', 'application/json; charset=utf-8');
      response.end(JSON.stringify(report));
      return;
    }
    const file = files[path];
    if (file === undefined || !existsSync(file)) {
      response.statusCode = 404;
      response.end('not found');
      return;
    }
    try {
      const bytes = await readFile(file);
      response.setHeader('content-type', types[file.slice(file.lastIndexOf('.'))] ?? 'application/octet-stream');
      response.setHeader('cache-control', 'no-store');
      response.end(bytes);
    } catch (error) {
      response.statusCode = 500;
      response.end(String(error));
    }
  });
  return new Promise((resolve, reject) => {
    server.on('error', reject);
    server.listen(port, '127.0.0.1', () => {
      const actual = server.address().port;
      resolve({
        port: actual,
        url: `http://127.0.0.1:${actual}/pagination.html`,
        lastReport: () => report,
        close: () => new Promise((done) => server.close(() => done())),
      });
    });
  });
}

// Direct invocation: keep serving until interrupted.
if (process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const requested = Number(process.argv[2] ?? 0);
  const started = await startServer(Number.isFinite(requested) ? requested : 0, (report) => {
    console.log(`REPORT ${JSON.stringify(report)}`);
  });
  console.log(`pagination harness: ${started.url}`);
}
