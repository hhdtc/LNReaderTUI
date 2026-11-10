/**
 * HTTP surface of the novel-reader panel.
 *
 * Every route lives under `/novel-reader/api` on the DSH Web listener, so the
 * panel's `fetch` calls are same-origin. Reading and library mutations are
 * answered in-process from the shared LNReaderTUI data directory; anything that
 * reaches bilinovel.com is delegated to the `lnreadertui` CLI, either
 * synchronously (search, detail) or as a cancellable job (download, update,
 * import).
 */

import { stat } from 'node:fs/promises';
import { readBookCover, readBookFile, readBookImage } from './book.js';
import { createCli } from './cli.js';
import { readSettings, saveSettings } from './settings.js';
import {
  bookFilePath,
  deleteBook,
  findBook,
  listBooks,
  readLibrary,
  resetProgress,
  resolveDataDir,
  setProgress,
} from './library.js';

export const API_PREFIX = '/novel-reader/api';

const MAX_BODY_BYTES = 64 * 1024;
const SEARCH_TIMEOUT_MS = 60000;
const DETAIL_TIMEOUT_MS = 90000;
const MAX_CHAPTER_BYTES = 2 * 1024 * 1024;
const BOOK_CACHE_LIMIT = 4;

/** Parsed books kept hot, keyed by file path and invalidated by mtime/size. */
const bookCache = new Map();

/**
 * Create the request handler shared by every route.
 * @param {{dataDir: string, bin?: string}} options
 * @returns {{handle: (request: import('node:http').IncomingMessage, response: import('node:http').ServerResponse) => Promise<void>, dispose: () => void, cli: any}}
 */
export function createNovelReader(options) {
  const dataDir = resolveDataDir(options.dataDir);
  const cli = createCli({ dataDir, bin: options.bin });
  const jobs = new Map();

  /**
   * Parse a book file once per (path, mtime, size).
   * @param {string} path
   * @returns {Promise<{title: string, author: string, chapters: {title: string, paragraphs: string[]}[]}>}
   */
  async function loadBook(path) {
    const cached = bookCache.get(path);
    if (cached !== undefined) {
      const info = await statOrNull(path);
      if (info !== null && info.mtimeMs === cached.mtimeMs && info.size === cached.size) {
        bookCache.delete(path);
        bookCache.set(path, cached);
        return cached.book;
      }
      bookCache.delete(path);
    }
    const info = await statOrNull(path);
    if (info === null) {
      const error = new Error(`book file is missing: ${path}`);
      error.code = 'BOOK_MISSING';
      throw error;
    }
    const book = await readBookFile(path);
    bookCache.set(path, { mtimeMs: info.mtimeMs, size: info.size, book });
    while (bookCache.size > BOOK_CACHE_LIMIT) {
      const oldest = bookCache.keys().next().value;
      bookCache.delete(oldest);
    }
    return book;
  }

  /**
   * Load one library book and its parsed content.
   * @param {string} id
   */
  async function loadLibraryBook(id) {
    const book = await findBook(dataDir, id);
    if (book === null) {
      const error = new Error(`book not found: ${id}`);
      error.code = 'BOOK_NOT_FOUND';
      throw error;
    }
    const path = bookFilePath(dataDir, book);
    const parsed = await loadBook(path);
    return { book, parsed, path };
  }

  const binaryRoutes = [
    ['GET', /^\/books\/([^/]+)\/image$/, (match, url) => bookImage(decodeURIComponent(match[1]), url.searchParams.get('path') ?? '')],
    ['GET', /^\/books\/([^/]+)\/cover$/, (match) => bookCover(decodeURIComponent(match[1]))],
  ];

  const routes = [
    ['GET', /^\/status$/, () => status()],
    ['GET', /^\/reader-settings$/, () => readSettings(dataDir)],
    ['POST', /^\/reader-settings$/, (_match, body) => saveSettings(dataDir, body)],
    ['GET', /^\/books$/, () => books()],
    ['GET', /^\/books\/([^/]+)\/chapters$/, (match) => chapters(decodeURIComponent(match[1]))],
    ['GET', /^\/books\/([^/]+)\/chapter\/(\d+)$/, (match) => chapter(decodeURIComponent(match[1]), Number(match[2]))],
    ['POST', /^\/books\/([^/]+)\/progress$/, (match, body) => saveProgress(decodeURIComponent(match[1]), body)],
    ['POST', /^\/books\/([^/]+)\/reset$/, (match) => resetProgressRoute(decodeURIComponent(match[1]))],
    ['POST', /^\/books\/([^/]+)\/delete$/, (match) => deleteRoute(decodeURIComponent(match[1]))],
    ['POST', /^\/import$/, (_match, body) => importRoute(body)],
    ['GET', /^\/search$/, (_match, _body, url) => search(url.searchParams.get('q') ?? '')],
    ['GET', /^\/detail$/, (_match, _body, url) => detail(url.searchParams.get('q') ?? '')],
    ['POST', /^\/download$/, (_match, body) => download(body)],
    ['POST', /^\/update$/, (_match, body) => update(body)],
    ['GET', /^\/jobs$/, () => listJobs()],
    ['GET', /^\/jobs\/([^/]+)$/, (match) => job(match[1])],
    ['POST', /^\/jobs\/([^/]+)\/cancel$/, (match) => cancelJobRoute(match[1])],
  ];

  async function status() {
    const cliStatus = await cli.status();
    let bookCount = 0;
    let libraryError = null;
    try {
      bookCount = (await listBooks(dataDir)).length;
    } catch (error) {
      libraryError = error instanceof Error ? error.message : String(error);
    }
    return {
      dataDir,
      cli: cliStatus,
      books: bookCount,
      libraryError,
      runningJobs: [...jobs.values()].filter((entry) => entry.status === 'running').length,
    };
  }

  async function books() {
    return { dataDir, books: await listBooks(dataDir) };
  }

  async function chapters(id) {
    const { book, parsed } = await loadLibraryBook(id);
    return {
      book: {
        id: book.id,
        title: parsed.title || book.title,
        author: parsed.author || book.author,
        chapters: parsed.chapters.length,
        storedChapters: book.chapters,
        fileType: book.fileType,
        sourceId: book.sourceId ?? '',
      },
      progress: book.progress ?? null,
      chapters: parsed.chapters.map((entry, index) => ({ index, title: entry.title })),
    };
  }

  async function chapter(id, index) {
    const { book, parsed, path } = await loadLibraryBook(id);
    if (!Number.isInteger(index) || index < 0 || index >= parsed.chapters.length) {
      const error = new Error(`chapter out of range: ${index}`);
      error.code = 'CHAPTER_NOT_FOUND';
      throw error;
    }
    const entry = parsed.chapters[index];
    let paragraphs = entry.paragraphs;
    let truncated = false;
    let total = 0;
    for (const paragraph of paragraphs) total += paragraph.length;
    if (total > MAX_CHAPTER_BYTES) {
      // A single pathological chapter: cap the payload rather than the read.
      const kept = [];
      let bytes = 0;
      for (const paragraph of paragraphs) {
        if (bytes + paragraph.length > MAX_CHAPTER_BYTES) {
          truncated = true;
          break;
        }
        kept.push(paragraph);
        bytes += paragraph.length;
      }
      paragraphs = kept;
    }
    return {
      id: book.id,
      bookTitle: parsed.title || book.title,
      index,
      total: parsed.chapters.length,
      title: entry.title,
      paragraphs,
      // Reading order including illustration blocks; `paragraphs` stays the
      // text-only view (the terminal reader's shape) for compatibility.
      blocks: blocksOf(entry, path, truncated),
      truncated,
      progress: book.progress ?? null,
    };
  }

  /**
   * Blocks of one chapter with only the images this book can actually serve.
   * @param {any} entry - parsed chapter.
   * @param {string} path - book file, so only .epub assets are offered.
   * @param {boolean} truncated
   */
  function blocksOf(entry, path, truncated) {
    const servesImages = String(path).toLowerCase().endsWith('.epub') && !truncated;
    const blocks = Array.isArray(entry.blocks) ? entry.blocks : entry.paragraphs.map((text) => ({ type: 'text', text }));
    if (!servesImages) return blocks.map((block) => (block.type === 'image' ? { type: 'text', text: '[图]' } : block));
    return blocks;
  }

  /** Serve one illustration out of an EPUB. */
  async function bookImage(id, name) {
    const entry = String(name).trim();
    if (entry === '' || entry.startsWith('/') || entry.split('/').includes('..')) {
      const error = new Error('path must be a relative archive entry');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    const { path } = await loadLibraryBook(id);
    if (!String(path).toLowerCase().endsWith('.epub')) {
      const error = new Error('this book has no illustrations');
      error.code = 'IMAGE_NOT_FOUND';
      throw error;
    }
    const asset = await readBookImage(path, entry);
    if (asset === null) {
      const error = new Error(`no such image: ${entry}`);
      error.code = 'IMAGE_NOT_FOUND';
      throw error;
    }
    return { ...asset, cacheSeconds: 3600 };
  }

  /** Serve a book's cover image. */
  async function bookCover(id) {
    const { path } = await loadLibraryBook(id);
    const asset = String(path).toLowerCase().endsWith('.epub') ? await readBookCover(path) : null;
    if (asset === null) {
      const error = new Error('this book has no cover');
      error.code = 'IMAGE_NOT_FOUND';
      throw error;
    }
    return { ...asset, cacheSeconds: 3600 };
  }

  async function saveProgress(id, body) {
    const chapterIndex = Number(body?.chapterIndex);
    const offset = Number(body?.offset);
    if (!Number.isFinite(chapterIndex) || !Number.isFinite(offset)) {
      const error = new Error('chapterIndex and offset are required numbers');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    const progress = await setProgress(dataDir, id, {
      chapterIndex,
      offset,
      chapterTitle: typeof body?.chapterTitle === 'string' ? body.chapterTitle : '',
    });
    return { progress };
  }

  async function resetProgressRoute(id) {
    return await resetProgress(dataDir, id);
  }

  async function deleteRoute(id) {
    // A deleted file can never be re-stated, so drop every parsed copy rather
    // than guess which cache key belonged to this id.
    bookCache.clear();
    return await deleteBook(dataDir, id);
  }

  async function importRoute(body) {
    const path = typeof body?.path === 'string' ? body.path.trim() : '';
    if (path === '') {
      const error = new Error('path is required');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    const result = await cli.run(['import', path, '--json'], { timeoutMs: 600000 });
    return { imported: Array.isArray(result.json) ? result.json : [] };
  }

  async function search(query) {
    const text = query.trim();
    if (text === '') return { total: 0, books: [], suggestion: '' };
    const result = await cli.run(['search', text, '--json'], { timeoutMs: SEARCH_TIMEOUT_MS });
    const payload = result.json ?? {};
    const items = Array.isArray(payload.Books) ? payload.Books : [];
    return {
      total: Number(payload.Total) || items.length,
      suggestion: typeof payload.Suggestion === 'string' ? payload.Suggestion : '',
      books: items.map(normalizeSiteBook),
    };
  }

  async function detail(query) {
    const text = query.trim();
    if (text === '') {
      const error = new Error('query is required');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    const result = await cli.run(['detail', text, '--json'], { timeoutMs: DETAIL_TIMEOUT_MS });
    const payload = result.json ?? {};
    const volumes = Array.isArray(payload.volumes) ? payload.volumes : [];
    const novelId = novelIdOf(String(payload.url ?? ''));
    const library = await readLibrary(dataDir).catch(() => ({ books: [] }));
    const existing = novelId === ''
      ? null
      : library.books.find((book) => book?.sourceId === novelId) ?? null;
    return {
      book: normalizeSiteBook(payload),
      totalChapters: Number(payload.totalChapters) || volumes.reduce((sum, volume) => sum + (Array.isArray(volume?.Chapters) ? volume.Chapters.length : 0), 0),
      volumes: volumes.map((volume) => ({
        title: typeof volume?.Title === 'string' ? volume.Title : '',
        chapters: (Array.isArray(volume?.Chapters) ? volume.Chapters : []).map((entry) => ({
          title: typeof entry?.Title === 'string' ? entry.Title : '',
        })),
      })),
      downloaded: existing === null ? null : { id: existing.id, title: existing.title, progress: existing.progress ?? null },
    };
  }

  async function download(body) {
    const query = typeof body?.query === 'string' ? body.query.trim() : '';
    if (query === '') {
      const error = new Error('query is required');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    return startJob({ kind: 'download', label: query, args: ['download', query, '--json'], query });
  }

  async function update(body) {
    const query = typeof body?.query === 'string' ? body.query.trim() : '';
    if (query === '') {
      const error = new Error('query is required');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    return startJob({ kind: 'update', label: query, args: ['update', query, '--json'], query });
  }

  async function startJob(request) {
    for (const existing of jobs.values()) {
      if (existing.status === 'running' && existing.query === request.query && existing.kind === request.kind) {
        return publicJob(existing);
      }
    }
    const created = await cli.startJob(request);
    jobs.set(created.id, created);
    // Keep the table bounded: finished jobs linger for the UI, oldest first out.
    const finished = [...jobs.values()].filter((entry) => entry.status !== 'running');
    if (finished.length > 20) {
      for (const entry of finished.slice(0, finished.length - 20)) jobs.delete(entry.id);
    }
    return publicJob(created);
  }

  function listJobs() {
    return { jobs: [...jobs.values()].map(publicJob) };
  }

  function job(id) {
    const found = jobs.get(id);
    if (found === undefined) {
      const error = new Error(`job not found: ${id}`);
      error.code = 'JOB_NOT_FOUND';
      throw error;
    }
    return publicJob(found);
  }

  function cancelJobRoute(id) {
    const found = jobs.get(id);
    if (found === undefined) {
      const error = new Error(`job not found: ${id}`);
      error.code = 'JOB_NOT_FOUND';
      throw error;
    }
    const cancelled = cli.cancelJob(found);
    return { id, cancelled, job: publicJob(found) };
  }

  function publicJob(entry) {
    return {
      id: entry.id,
      kind: entry.kind,
      label: entry.label,
      status: entry.status,
      startedAt: entry.startedAt,
      endedAt: entry.endedAt,
      exitCode: entry.exitCode,
      progress: entry.progress,
      result: entry.result,
      error: entry.error,
      logs: entry.logs.slice(-40),
    };
  }

  /**
   * Answer one request: same-origin only, JSON in/out, no caching.
   * @param {import('node:http').IncomingMessage} request
   * @param {import('node:http').ServerResponse} response
   */
  async function handle(request, response) {
    const url = new URL(String(request.url ?? '/'), 'http://localhost');
    if (!url.pathname.startsWith(API_PREFIX)) {
      sendJson(response, 404, { error: { code: 'NOT_FOUND', message: 'unknown route' } });
      return;
    }
    const path = url.pathname.slice(API_PREFIX.length) || '/';
    if (request.method === 'GET') {
      for (const [method, pattern, run] of binaryRoutes) {
        if (request.method !== method) continue;
        const match = pattern.exec(path);
        if (match === null) continue;
        try {
          const asset = await run(match, url);
          response.statusCode = 200;
          response.setHeader('content-type', asset.contentType);
          response.setHeader('cache-control', `private, max-age=${asset.cacheSeconds ?? 600}`);
          response.setHeader('content-length', String(asset.bytes.length));
          response.end(asset.bytes);
        } catch (error) {
          sendJson(response, errorStatus(error), errorBody(error));
        }
        return;
      }
    }
    for (const [method, pattern, run] of routes) {
      if (request.method !== method) continue;
      const match = pattern.exec(path);
      if (match === null) continue;
      try {
        const body = method === 'POST' ? await readJsonBody(request) : null;
        const payload = await run(match, body, url);
        sendJson(response, 200, payload);
      } catch (error) {
        sendJson(response, errorStatus(error), errorBody(error));
      }
      return;
    }
    sendJson(response, 405, { error: { code: 'METHOD_NOT_ALLOWED', message: `${request.method} ${path}` } });
  }

  function dispose() {
    for (const entry of jobs.values()) {
      if (entry.child !== null) {
        try {
          entry.child.kill('SIGKILL');
        } catch { /* already gone */ }
      }
    }
    jobs.clear();
    bookCache.clear();
  }

  return { handle, dispose, cli, dataDir };
}

/**
 * Normalize one CLI search/detail book record (`cover_url` snake case) into the
 * panel's wire shape.
 * @param {any} book
 */
function normalizeSiteBook(book) {
  const url = typeof book?.url === 'string' ? book.url : '';
  return {
    title: typeof book?.title === 'string' ? book.title : '',
    author: typeof book?.author === 'string' ? book.author : '',
    publisher: typeof book?.publisher === 'string' ? book.publisher : '',
    status: typeof book?.status === 'string' ? book.status : '',
    rating: typeof book?.rating === 'string' ? book.rating : '',
    tags: typeof book?.tags === 'string' ? book.tags : '',
    description: typeof book?.description === 'string' ? book.description : '',
    url,
    coverUrl: typeof book?.cover_url === 'string' ? book.cover_url : '',
    novelId: novelIdOf(url),
  };
}

/** Novel id from a bilinovel/linovelib URL (the CLI's own rule). */
export function novelIdOf(url) {
  const match = /(?:linovelib|bilinovel)\.(?:com|net)\/(?:novel|download)\/(\d+)/.exec(url);
  return match === null ? '' : match[1];
}

async function statOrNull(path) {
  try {
    return await stat(path);
  } catch {
    return null;
  }
}

/** Collect a bounded JSON request body; an empty body reads as `{}`. */
async function readJsonBody(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.byteLength;
    if (size > MAX_BODY_BYTES) {
      request.resume();
      const error = new Error('request body too large');
      error.code = 'BAD_REQUEST';
      throw error;
    }
    chunks.push(chunk);
  }
  if (size === 0) return {};
  const text = Buffer.concat(chunks).toString('utf8');
  try {
    const parsed = JSON.parse(text);
    return parsed !== null && typeof parsed === 'object' ? parsed : {};
  } catch {
    const error = new Error('request body is not JSON');
    error.code = 'BAD_REQUEST';
    throw error;
  }
}

/** HTTP status for one typed failure. */
function errorStatus(error) {
  const code = typeof error?.code === 'string' ? error.code : 'INTERNAL';
  if (code === 'BAD_REQUEST') return 400;
  if (code === 'BOOK_NOT_FOUND' || code === 'CHAPTER_NOT_FOUND' || code === 'JOB_NOT_FOUND' || code === 'IMAGE_NOT_FOUND') return 404;
  if (code === 'BOOK_MISSING' || code === 'CLI_MISSING') return 409;
  if (code === 'CLI_FAILED') return 502;
  return 500;
}

/** JSON error body for one thrown failure. */
function errorBody(error) {
  return {
    error: {
      code: typeof error?.code === 'string' ? error.code : 'INTERNAL',
      message: error instanceof Error ? error.message : String(error),
    },
  };
}

function sendJson(response, status, payload) {
  if (response.headersSent) return;
  response.statusCode = status;
  response.setHeader('content-type', 'application/json; charset=utf-8');
  response.setHeader('cache-control', 'no-store');
  response.end(`${JSON.stringify(payload)}\n`);
}
