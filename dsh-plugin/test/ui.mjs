/**
 * Interaction test for the browser half of the panel.
 *
 * Renders the real page components with React 18 inside jsdom against a fake
 * `/novel-reader/api`, then drives the flows a reader uses: library list →
 * open a book → read a chapter → page/chapter turns → saved progress,
 * sidebar catalog, detail view and job progress.
 *
 * Dev-only dependencies (react, react-dom, jsdom) live in `.reacttest`; when it
 * is absent the script reports how to create it instead of failing.
 *
 *   npm install --no-save --prefix .reacttest react@18.3.1 react-dom@18.3.1 jsdom@25
 *   node test/ui.mjs
 */

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const pluginRoot = join(here, '..');
const devModules = join(pluginRoot, '.reacttest', 'node_modules');

if (!existsSync(join(devModules, 'react-dom')) || !existsSync(join(devModules, 'jsdom'))) {
  console.log('skip: run `npm install --no-save --prefix .reacttest react@18.3.1 react-dom@18.3.1 jsdom@25` first');
  process.exit(0);
}

// The DOM has to exist before React DOM is imported: React probes
// `'oninput' in document` once at module load to decide whether the browser
// fires native input events, and jsdom only answers true for a defined handler.
const { JSDOM } = await import(pathToFileURL(join(devModules, 'jsdom', 'lib', 'api.js')).href);
const dom = new JSDOM('<!doctype html><html><head></head><body><div id="root"></div></body></html>', {
  url: 'http://127.0.0.1:3080/',
  pretendToBeVisual: true,
});
const { window } = dom;
Object.defineProperty(window.document, 'oninput', { value: null, writable: true, configurable: true });
// jsdom has no layout engine: give the reader's scroll calls somewhere to land.
window.Element.prototype.scrollIntoView = function scrollIntoView() {};
window.Element.prototype.scrollTo = function scrollTo() {};
globalThis.window = window;
globalThis.document = window.document;
// The panel picks its copy from navigator.language at module evaluation.
Object.defineProperty(window.navigator, 'language', { value: 'zh-CN', configurable: true });
Object.defineProperty(globalThis, 'navigator', { value: window.navigator, configurable: true });
globalThis.HTMLElement = window.HTMLElement;
globalThis.requestAnimationFrame = (callback) => window.setTimeout(() => callback(Date.now()), 0);
globalThis.cancelAnimationFrame = (id) => window.clearTimeout(id);
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const React = (await import(pathToFileURL(join(devModules, 'react', 'index.js')).href)).default;
const ReactDOM = await import(pathToFileURL(join(devModules, 'react-dom', 'index.js')).href);
const act = React.act ?? (await import(pathToFileURL(join(devModules, 'react-dom', 'test-utils.js')).href)).act;

/** Requests the fake API received, in order. */
const calls = [];
const savedProgress = [];

const BOOK = {
  id: 'abc123',
  title: '月之珊瑚',
  author: '奈须蘑菇',
  file: 'books/月之珊瑚-764.epub',
  fileType: 'epub',
  chapters: 3,
  sourceId: '764',
  sourceUrl: 'https://www.bilinovel.com/novel/764.html',
  present: true,
  sizeBytes: 50762,
  progress: { chapterIndex: 1, offset: 0.5, chapterTitle: '第二章', lastReadAt: '2026-09-01T10:00:00Z' },
  summary: { read: true, label: '第二章 50%' },
};

const CHAPTERS = [
  { index: 0, title: '第一章' },
  { index: 1, title: '第二章' },
  { index: 2, title: '第三章' },
];

function chapterBody(index) {
  return {
    id: BOOK.id,
    bookTitle: BOOK.title,
    index,
    total: CHAPTERS.length,
    title: CHAPTERS[index].title,
    paragraphs: [`第${index + 1}章 第一段`, '第二段文字', '第三段文字'],
    truncated: false,
    progress: BOOK.progress,
  };
}

let jobPolls = 0;

function route(url, init) {
  const path = new URL(String(url), 'http://127.0.0.1:3080').pathname.replace('/novel-reader/api', '');
  calls.push({ path, method: init?.method ?? 'GET' });
  const method = init?.method ?? 'GET';
  const body = init?.body ? JSON.parse(init.body) : null;
  if (path === '/status') {
    return {
      dataDir: '/home/reader/.local/share/lnreadertui',
      cli: { available: true, path: '/usr/bin/lnreadertui', version: 'lnreadertui dev' },
      books: 1,
      runningJobs: 0,
    };
  }
  if (path === '/books') return { books: [BOOK] };
  if (path === `/books/${BOOK.id}/chapters`) return { chapters: CHAPTERS, progress: BOOK.progress, book: BOOK };
  const chapterMatch = /^\/books\/([^/]+)\/chapter\/(\d+)$/.exec(path);
  if (chapterMatch !== null) return chapterBody(Number(chapterMatch[2]));
  if (path === `/books/${BOOK.id}/progress` && method === 'POST') {
    savedProgress.push(body);
    return { progress: { ...body, lastReadAt: '2026-09-02T10:00:00Z' } };
  }
  if (path === '/search') {
    return {
      total: 1,
      suggestion: '',
      books: [{
        title: '月之珊瑚',
        author: '奈须蘑菇',
        status: '完结',
        rating: '9.0',
        publisher: '',
        tags: '短篇',
        description: '月与珊瑚的短篇故事。',
        url: 'https://www.bilinovel.com/novel/764.html',
        coverUrl: '',
        novelId: '764',
      }],
    };
  }
  if (path === '/detail') {
    return {
      book: {
        title: '月之珊瑚',
        author: '奈须蘑菇',
        status: '完结',
        rating: '9.0',
        publisher: '',
        tags: '短篇',
        description: '月与珊瑚的短篇故事。',
        url: 'https://www.bilinovel.com/novel/764.html',
        coverUrl: '',
        novelId: '764',
      },
      totalChapters: 3,
      volumes: [{ title: '第一卷', chapters: [{ title: '第一章' }, { title: '第二章' }, { title: '第三章' }] }],
      downloaded: { id: BOOK.id, title: BOOK.title, progress: BOOK.progress },
    };
  }
  if (path === '/download' && method === 'POST') {
    return {
      id: 'job1',
      kind: 'download',
      label: body?.query ?? '',
      status: 'running',
      progress: { done: 2, total: 5, volume: '第一卷', title: '第二章' },
      logs: [{ at: '', stream: 'err', text: 'fetching chapter' }],
    };
  }
  if (path === '/jobs/job1') {
    jobPolls += 1;
    return {
      id: 'job1',
      kind: 'download',
      label: '764',
      status: jobPolls > 1 ? 'done' : 'running',
      progress: { done: jobPolls > 1 ? 5 : 3, total: 5, volume: '第一卷', title: '第三章' },
      result: jobPolls > 1 ? { savedTo: '/home/reader/.local/share/lnreadertui/books/月之珊瑚-764.epub' } : null,
      logs: [],
    };
  }
  if (path === '/jobs/job1/cancel' && method === 'POST') {
    return { id: 'job1', cancelled: true, job: { id: 'job1', kind: 'download', status: 'cancelled', progress: {}, logs: [] } };
  }
  throw new Error(`unexpected request ${method} ${path}`);
}

globalThis.fetch = async (url, init) => {
  try {
    const payload = route(url, init);
    return { ok: true, status: 200, statusText: 'OK', json: async () => payload };
  } catch (error) {
    return { ok: false, status: 500, statusText: error.message, json: async () => ({ error: { message: error.message } }) };
  }
};

// ── load the plugin bundle the way the DSH module loader does ───────────────

const source = await readFile(join(pluginRoot, 'lib', 'client.js'), 'utf8');
const registered = [];
// The bundle is a classic script: in the browser `window` is both the loader
// host and the DOM window, so the harness has to be the same object.
window.__ModuleLoader__ = { load: (entry) => registered.push(entry) };
const requireShim = (id) => {
  assert.equal(id, 'react', `bundle requested unexpected module ${id}`);
  return React;
};
new Function('window', 'document', 'navigator', 'fetch', source)(window, window.document, window.navigator, globalThis.fetch);
const plugin = registered[0].factory(requireShim);
assert.equal(registered[0].id, 'dsh-novel-reader');

const registrations = [];
const ctx = {
  effect: (body) => { body(); return () => {}; },
  slots: {
    inject: (name, body) => body(),
    register: (options, component) => {
      registrations.push({ options, component });
      return () => {};
    },
  },
};
plugin.apply(ctx);
const mainRegistration = registrations.find((entry) => entry.options.name === 'main');
const launcherRegistration = registrations.find((entry) => entry.options.name === 'sidebar.panellist');
assert.equal(mainRegistration.options.key, 'novel-reader');
assert.equal(launcherRegistration.options.id, 'novel-reader');
assert.equal(launcherRegistration.options.label(), '小说');

// ── render and drive the page ───────────────────────────────────────────────

const container = window.document.getElementById('root');
const root = ReactDOM.createRoot(container);

const flush = async (ms = 0) => {
  await act(async () => {
    await new Promise((resolve) => window.setTimeout(resolve, ms));
  });
};
const text = () => container.textContent ?? '';
const click = async (element) => {
  assert.ok(element !== null && element !== undefined, 'expected an element to click');
  await act(async () => {
    element.dispatchEvent(new window.MouseEvent('click', { bubbles: true }));
  });
};
const findButton = (label, scope = container) => [...scope.querySelectorAll('button')]
  .find((button) => (button.textContent ?? '').includes(label));

let failures = 0;
async function step(label, body) {
  try {
    await body();
    console.log(`ok   ${label}`);
  } catch (error) {
    failures += 1;
    console.log(`FAIL ${label}\n     ${error instanceof Error ? error.message.split('\n').join('\n     ') : String(error)}`);
  }
}

await act(async () => {
  root.render(React.createElement(mainRegistration.component));
});
await flush(20);

await step('library renders the shared book with its reading position', async () => {
  assert.ok(text().includes('小说阅读'), `missing page title: ${text().slice(0, 120)}`);
  assert.ok(text().includes('月之珊瑚'));
  assert.ok(text().includes('奈须蘑菇'));
  assert.ok(text().includes('第 2/3 章'), `missing progress line: ${text().slice(0, 400)}`);
  assert.ok(text().includes('/home/reader/.local/share/lnreadertui'));
  assert.ok(text().includes('lnreadertui dev'));
  assert.ok(text().includes('50%'));
});

await step('opening the book loads chapter text in a paged (non-scrolling) view', async () => {
  const card = [...container.querySelectorAll('.nr-card')].find((element) => (element.textContent ?? '').includes('月之珊瑚'));
  await click(card);
  await flush(40);
  assert.ok(text().includes('第一段'), `chapter text missing: ${text().slice(0, 200)}`);
  assert.ok(text().includes('第二章'), 'chapter title missing');
  assert.ok(text().includes('本章'), 'reader footer missing');
  const viewport = container.querySelector('.nr-reader-viewport');
  assert.ok(viewport !== null, 'reading viewport missing');
  // Page mode: a column strip is translated, nothing scrolls the text.
  const flow = container.querySelector('.nr-reader-flow');
  assert.ok(flow !== null, 'column strip missing');
  assert.ok((flow.getAttribute('style') ?? '').includes('column-width'), 'strip is not column-paged');
  assert.ok((flow.getAttribute('style') ?? '').includes('translate3d'), 'strip has no page translation');
  assert.equal(container.querySelector('.nr-reader-scroll'), null, 'the scrolling reader is gone');
  assert.ok(text().includes('1 / 1'), `page indicator missing: ${text().slice(-120)}`);
  assert.ok(calls.some((entry) => entry.path === '/books/abc123/chapter/1'));
});

await step('the chapter catalog lists every chapter and jumps', async () => {
  await click(findButton('目录'));
  await flush(10);
  const items = [...container.querySelectorAll('.nr-chapter')];
  assert.equal(items.length, 3);
  assert.equal(items[1].getAttribute('data-current'), 'true');
  await click(items[2]);
  await flush(40);
  assert.ok(text().includes('第三段文字'));
  assert.ok(calls.some((entry) => entry.path === '/books/abc123/chapter/2'));
});

await step('page and chapter keys request the neighbouring chapter', async () => {
  await act(async () => {
    window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
  });
  await flush(20);
  await act(async () => {
    window.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'p', bubbles: true }));
  });
  await flush(40);
  const chapterCalls = calls.filter((entry) => entry.path.startsWith('/books/abc123/chapter/')).map((entry) => entry.path);
  // A viewport-less jsdom always reads "at the bottom", so ArrowRight at the
  // last chapter clamps and `p` walks back: the walk-back must re-fetch.
  assert.ok(chapterCalls.length >= 3, `chapter requests: ${chapterCalls.join(', ')}`);
  assert.equal(chapterCalls[chapterCalls.length - 1], '/books/abc123/chapter/1');
});

await step('a reading position is written back to the shared library', async () => {
  await flush(1200);
  assert.ok(savedProgress.length > 0, 'no progress POST observed');
  const last = savedProgress[savedProgress.length - 1];
  assert.equal(typeof last.chapterIndex, 'number');
  assert.ok(last.offset >= 0 && last.offset <= 1);
  assert.equal(typeof last.chapterTitle, 'string');
});

await step('leaving the reader returns to the refreshed library', async () => {
  await click(findButton('返回书库'));
  await flush(40);
  assert.ok(text().includes('书库'));
  assert.ok(text().includes('月之珊瑚'));
});

await step('search shows results and the library badge', async () => {
  await click(findButton('搜索', container.querySelector('.nr-tabs')));
  await flush(20);
  const input = container.querySelector('.nr-input');
  assert.ok(input !== null);
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    setter.call(input, '月之珊瑚');
    input.dispatchEvent(new window.Event('input', { bubbles: true }));
  });
  await flush(700);
  assert.ok(calls.some((entry) => entry.path === '/search'), 'search request missing');
  assert.ok(text().includes('月之珊瑚'));
  assert.ok(text().includes('已在书库'));
});

await step('starting a download streams job progress and settles', async () => {
  const download = findButton('下载');
  assert.ok(download !== undefined, 'download button missing');
  await click(download);
  await flush(20);
  assert.ok(calls.some((entry) => entry.path === '/download' && entry.method === 'POST'));
  assert.ok(text().includes('下载中') || text().includes('进行中'));
  // Each act() flushes one render, so let the polling effect reschedule a few
  // times instead of sleeping through a single long batch.
  for (let i = 0; i < 8 && !text().includes('已完成'); i += 1) await flush(800);
  assert.ok(jobPolls >= 2, `polls: ${jobPolls}`);
  assert.ok(text().includes('已完成') || text().includes('已保存到'), `job did not settle: ${text().slice(-400)}`);
});

await step('detail view renders volumes and the in-library state', async () => {
  const detailButton = [...container.querySelectorAll('button')].find((button) => button.textContent === '详情');
  assert.ok(detailButton !== undefined, 'detail button missing');
  await click(detailButton);
  await flush(60);
  assert.ok(text().includes('第一卷'));
  assert.ok(text().includes('3 章') || text().includes('3 卷'));
  assert.ok(calls.some((entry) => entry.path === '/detail'));
});

await act(async () => {
  root.unmount();
});
await flush(10);

if (failures > 0) {
  console.error(`\n${failures} UI check(s) failed`);
  process.exit(1);
}
console.log('\nall UI checks passed');
