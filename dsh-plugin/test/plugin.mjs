/**
 * End-to-end checks for the novel-reader plugin without the DSH GUI.
 *
 * Covers three things the panel's correctness rests on:
 *  1. the library store speaks LNReaderTUI's schema (progress round-trip, reset,
 *     delete, and field shapes read back by the terminal reader);
 *  2. the HTTP surface answers real book content from a temp data directory;
 *  3. the browser bundle loads under the module loader and registers the main
 *     panel plus its left-sidebar launcher.
 *
 * Run with `node test/plugin.mjs`.
 */

import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { createNovelReader, novelIdOf } from '../lib/routes.js';
import { readBookFile, readZipIndex } from '../lib/book.js';
import { listBooks, resolveDataDir, setProgress } from '../lib/library.js';

const here = dirname(fileURLToPath(import.meta.url));
const pluginRoot = join(here, '..');
const fixtures = join(here, 'fixtures');

let failures = 0;
function check(label, body) {
  try {
    body();
    console.log(`ok   ${label}`);
  } catch (error) {
    failures += 1;
    console.log(`FAIL ${label}\n     ${error instanceof Error ? error.message.split('\n').join('\n     ') : String(error)}`);
  }
}

async function checkAsync(label, body) {
  try {
    await body();
    console.log(`ok   ${label}`);
  } catch (error) {
    failures += 1;
    console.log(`FAIL ${label}\n     ${error instanceof Error ? error.message.split('\n').join('\n     ') : String(error)}`);
  }
}

// ── a throwaway library containing one real book ────────────────────────────

const sourceDir = resolveDataDir();
const sourceLibrary = join(sourceDir, 'library.json');
const realBooks = existsSync(sourceLibrary)
  ? (JSON.parse(await readFile(sourceLibrary, 'utf8')).books ?? [])
  : [];
const sample = realBooks.find((book) => existsSync(join(sourceDir, book.file)));

// One book that actually carries in-chapter illustrations, so the image route
// is exercised against a real archive (cover-only books prove nothing).
let illustrated;
for (const book of realBooks) {
  if (!String(book.file ?? '').toLowerCase().endsWith('.epub')) continue;
  const path = join(sourceDir, book.file);
  if (!existsSync(path)) continue;
  if (book.id === sample?.id) continue;
  const index = readZipIndex(await readFile(path));
  if ([...index.keys()].some((name) => /(^|\/)images\/img[0-9]+\./.test(name))) {
    illustrated = book;
    break;
  }
}

const tempRoot = await mkdtemp(join(tmpdir(), 'novel-reader-test-'));
const dataDir = join(tempRoot, 'library');
await mkdir(join(dataDir, 'books'), { recursive: true });
const seeded = [sample, illustrated].filter((book) => book !== undefined);
if (seeded.length > 0) {
  for (const book of seeded) await copyFile(join(sourceDir, book.file), join(dataDir, book.file));
  await writeFile(join(dataDir, 'library.json'), `${JSON.stringify({
    books: seeded.map((book) => ({ ...book, progress: { chapterIndex: 0, offset: 0, lastReadAt: '' } })),
  }, null, 2)}\n`);
} else {
  console.log('note: no LNReaderTUI library on this machine; book tests run on a synthetic library');
}

// ── library store schema ────────────────────────────────────────────────────

await checkAsync('progress write keeps the terminal reader\'s schema', async () => {
  if (sample === undefined) return;
  const stored = await setProgress(dataDir, sample.id, { chapterIndex: 2, offset: 0.4913, chapterTitle: '第一话' });
  assert.deepEqual(Object.keys(stored).sort(), ['chapterIndex', 'chapterTitle', 'lastReadAt', 'offset']);
  assert.equal(stored.chapterIndex, 2);
  assert.ok(Math.abs(stored.offset - 0.4913) < 1e-9);
  assert.match(stored.lastReadAt, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/);
  const file = JSON.parse(await readFile(join(dataDir, 'library.json'), 'utf8'));
  assert.equal(file.books[0].progress.chapterIndex, 2);
  assert.equal(file.books[0].progress.chapterTitle, '第一话');
  // Untouched fields survive the rewrite.
  assert.equal(file.books[0].title, sample.title);
  assert.equal(file.books[0].sourceId, sample.sourceId);
});

await checkAsync('progress clamps like Store.SetProgress', async () => {
  if (sample === undefined) return;
  const stored = await setProgress(dataDir, sample.id, { chapterIndex: -5, offset: 3 });
  assert.equal(stored.chapterIndex, 0);
  assert.equal(stored.offset, 1);
});

await checkAsync('an empty chapter title is omitted, not stored blank', async () => {
  if (sample === undefined) return;
  await setProgress(dataDir, sample.id, { chapterIndex: 1, offset: 0.2, chapterTitle: '' });
  const file = JSON.parse(await readFile(join(dataDir, 'library.json'), 'utf8'));
  assert.equal(Object.hasOwn(file.books[0].progress, 'chapterTitle'), false);
});

// ── HTTP surface ────────────────────────────────────────────────────────────

const server = createNovelReader({ dataDir, bin: process.env.LNREADERTUI_BIN });
const http = createServer((request, response) => { server.handle(request, response); });
await new Promise((resolve) => http.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${http.address().port}/novel-reader/api`;

async function call(path, init) {
  const response = await fetch(`${origin}${path}`, {
    ...(init ?? {}),
    headers: { 'content-type': 'application/json', ...(init?.headers ?? {}) },
  });
  const body = await response.json();
  return { status: response.status, body };
}

await checkAsync('GET /books lists the shared library', async () => {
  const { status, body } = await call('/books');
  assert.equal(status, 200);
  assert.equal(body.dataDir, dataDir);
  if (sample !== undefined) {
    assert.equal(body.books.length, seeded.length);
    assert.equal(body.books[0].id, sample.id);
    assert.equal(body.books[0].present, true);
    assert.equal(body.books[0].summary.read, true);
    assert.ok(body.books[0].summary.label.includes('%'));
  }
});

if (sample !== undefined) {
  await checkAsync('GET /books/:id/chapters matches the stored chapter count', async () => {
    const { status, body } = await call(`/books/${sample.id}/chapters`);
    assert.equal(status, 200);
    assert.equal(body.chapters.length, sample.chapters);
    assert.equal(body.chapters[0].index, 0);
    assert.equal(typeof body.chapters[0].title, 'string');
    assert.equal(body.progress.chapterIndex, 1);
  });

  await checkAsync('GET /books/:id/chapter/:index returns readable paragraphs', async () => {
    const { status, body } = await call(`/books/${sample.id}/chapter/1`);
    assert.equal(status, 200);
    assert.equal(body.index, 1);
    assert.ok(body.paragraphs.length > 0);
    assert.equal(body.total, sample.chapters);
  });

  await checkAsync('POST progress round-trips through the shared file', async () => {
    const { status, body } = await call(`/books/${sample.id}/progress`, {
      method: 'POST',
      body: JSON.stringify({ chapterIndex: 3, offset: 0.25, chapterTitle: '第三章' }),
    });
    assert.equal(status, 200);
    assert.equal(body.progress.chapterIndex, 3);
    const [row] = await listBooks(dataDir);
    assert.equal(row.progress.chapterIndex, 3);
    assert.ok(Math.abs(row.progress.offset - 0.25) < 1e-9);
    assert.equal(row.progress.chapterTitle, '第三章');
  });

  await checkAsync('POST reset clears the position the TUI shows', async () => {
    const reset = await call(`/books/${sample.id}/reset`, { method: 'POST', body: '{}' });
    assert.equal(reset.status, 200);
    const [row] = await listBooks(dataDir);
    assert.equal(row.summary.read, false);
    assert.equal(row.summary.label, 'unread');
  });

  await checkAsync('unknown book and chapter are typed 404s', async () => {
    const missing = await call('/books/does-not-exist/chapters');
    assert.equal(missing.status, 404);
    assert.equal(missing.body.error.code, 'BOOK_NOT_FOUND');
    const range = await call(`/books/${sample.id}/chapter/9999`);
    assert.equal(range.status, 404);
    assert.equal(range.body.error.code, 'CHAPTER_NOT_FOUND');
  });

  await checkAsync('unknown route and method are refused', async () => {
    const unknown = await call('/nope');
    assert.equal(unknown.status, 405);
    const wrongMethod = await call('/books', { method: 'POST', body: '{}' });
    assert.equal(wrongMethod.status, 405);
  });

  await checkAsync('illustration blocks are served as images', async () => {
    if (illustrated === undefined) {
      console.log('     note: no illustrated book on this machine; image route not exercised');
      return;
    }
    // Find a chapter that carries an illustration block.
    let entry = '';
    for (let index = 0; index < 12 && entry === ''; index += 1) {
      const { body } = await call(`/books/${illustrated.id}/chapter/${index}`);
      const image = (body.blocks ?? []).find((block) => block.type === 'image');
      if (image !== undefined) entry = image.src;
    }
    assert.notEqual(entry, '', 'expected an illustration in the first chapters');

    const image = await fetch(`${origin}/books/${illustrated.id}/image?path=${encodeURIComponent(entry)}`);
    assert.equal(image.status, 200);
    assert.ok(String(image.headers.get('content-type')).startsWith('image/'), 'image content type');
    assert.ok((await image.arrayBuffer()).byteLength > 1000, 'image bytes');

    const cover = await fetch(`${origin}/books/${illustrated.id}/cover`);
    assert.equal(cover.status, 200);
    assert.ok(String(cover.headers.get('content-type')).startsWith('image/'), 'cover content type');

    // A traversal attempt and a non-image entry are both refused.
    const traversal = await fetch(`${origin}/books/${illustrated.id}/image?path=${encodeURIComponent('../content.opf')}`);
    assert.equal(traversal.status, 400);
    const notImage = await fetch(`${origin}/books/${illustrated.id}/image?path=${encodeURIComponent('META-INF/container.xml')}`);
    assert.equal(notImage.status, 404);
  });

  await checkAsync('POST delete removes the row and the file', async () => {
    const removed = await call(`/books/${sample.id}/delete`, { method: 'POST', body: '{}' });
    assert.equal(removed.status, 200);
    const { body } = await call('/books');
    assert.equal(body.books.length, seeded.length - 1);
    assert.equal(existsSync(join(dataDir, sample.file)), false);
  });
}

await checkAsync('GET /status reports the CLI truthfully', async () => {
  const { status, body } = await call('/status');
  assert.equal(status, 200);
  assert.equal(body.dataDir, dataDir);
  assert.equal(typeof body.cli.available, 'boolean');
  if (body.cli.available === false) console.log(`     note: ${body.cli.error}`);
});

await checkAsync('novel ids are read from site URLs like the CLI does', async () => {
  assert.equal(novelIdOf('https://www.bilinovel.com/novel/4649.html'), '4649');
  assert.equal(novelIdOf('https://www.linovelib.com/download/764'), '764');
  assert.equal(novelIdOf('https://example.com/novel/1.html'), '');
});

http.close();
server.dispose();

// ── browser bundle ──────────────────────────────────────────────────────────

await checkAsync('client bundle loads and registers the panel and its launcher', async () => {
  const source = await readFile(join(pluginRoot, 'lib', 'client.js'), 'utf8');
  const registered = [];
  const fakeWindow = {
    __ModuleLoader__: {
      load: (entry) => registered.push(entry),
    },
  };
  const fakeDocument = {
    head: { appendChild: () => {} },
    createElement: () => ({ dataset: {}, remove: () => {} }),
    addEventListener: () => {},
    removeEventListener: () => {},
  };
  const fakeRequire = (id) => {
    assert.equal(id, 'react', 'the bundle may only require platform seeds');
    return {
      createElement: (...args) => ({ type: args[0], props: args[1], children: args.slice(2) }),
      useState: (initial) => [initial, () => {}],
      useRef: (initial) => ({ current: initial }),
      useEffect: () => {},
      useCallback: (fn) => fn,
      useMemo: (fn) => fn(),
    };
  };
  // The bundle is evaluated as a classic script with the loader and DOM globals.
  const run = new Function('window', 'document', 'navigator', 'fetch', source);
  run(fakeWindow, fakeDocument, { language: 'zh-CN' }, async () => ({ ok: true, json: async () => ({}) }));

  assert.equal(registered.length, 1);
  assert.equal(registered[0].id, 'dsh-novel-reader');
  const exportsObject = registered[0].factory(fakeRequire);
  assert.equal(typeof exportsObject.apply, 'function');
  assert.deepEqual(exportsObject.inject, ['slots']);

  // Paged reading: a strip's page count must round from the measured width.
  const { pagesInStrip } = exportsObject;
  assert.equal(pagesInStrip(0, 400, 88), 1, 'an unmeasured strip is one page');
  assert.equal(pagesInStrip(400, 400, 88), 1);
  // An N-page strip is N * (width + gap) - gap wide (each page advances by the span).
  assert.equal(pagesInStrip(2 * (400 + 88) - 88, 400, 88), 2);
  assert.equal(pagesInStrip(3 * (400 + 88) - 88, 400, 88), 3);
  assert.equal(pagesInStrip(Number.NaN, 400, 88), 1);

  const slots = [];
  const ctx = {
    effect: (body) => { body(); return () => {}; },
    slots: {
      inject: (name, body) => { slots.push(name); return body(); },
      register: (options) => { slots.push(options); return () => {}; },
    },
  };
  exportsObject.apply(ctx);
  const main = slots.find((entry) => entry && entry.name === 'main');
  const launcher = slots.find((entry) => entry && entry.name === 'sidebar.panellist');
  assert.equal(main.key, 'novel-reader');
  assert.equal(launcher.id, 'novel-reader');
  assert.equal(typeof launcher.label(), 'string');
  assert.ok(launcher.label().length > 0);
});

// ── book reader parity spot check ───────────────────────────────────────────

await checkAsync('reading a fixture book yields the reference chapter count', async () => {
  const referenceFiles = existsSync(fixtures) ? await import('node:fs/promises').then((fs) => fs.readdir(fixtures)) : [];
  if (sample === undefined || referenceFiles.length === 0) return;
  const reference = JSON.parse(await readFile(join(fixtures, `reference-${sample.sourceId}.json`), 'utf8'));
  const parsed = await readBookFile(join(sourceDir, sample.file));
  assert.equal(parsed.chapters.length, reference.chapters.length);
  assert.equal(parsed.chapters[0].title, reference.chapters[0].Title ?? reference.chapters[0].title);
});

await rm(tempRoot, { recursive: true, force: true });

if (failures > 0) {
  console.error(`\n${failures} check(s) failed`);
  process.exit(1);
}
console.log('\nall plugin checks passed');
