/**
 * Parity check: the plugin's chapter view against LNReaderTUI's own reader.
 *
 * `test/fixtures/reference-<novelId>.json` holds the Go reader's view
 * (`internal/epub/reader.go`) of each book: chapter titles, paragraph counts,
 * and a SHA-256 of every chapter's paragraph array. This script re-reads the
 * same books with the plugin's reader and reports the first divergence.
 * Run with `node test/parity.mjs`.
 *
 * The fixture directory is deliberately optional: when it is absent the script
 * falls back to checking each library book's chapter count against the
 * `chapters` recorded in `library.json`, which is written from the Go reader at
 * download/import time.
 */

import { readFile, readdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readBookFile } from '../lib/book.js';

const here = dirname(fileURLToPath(import.meta.url));
const pluginRoot = join(here, '..');
const repoRoot = join(pluginRoot, '..');
const fixtures = join(here, 'fixtures');

function dataDir() {
  if (process.env.LNREADERTUI_DATA_DIR) return process.env.LNREADERTUI_DATA_DIR;
  if (process.env.XDG_DATA_HOME) return join(process.env.XDG_DATA_HOME, 'lnreadertui');
  if (process.platform === 'win32' && process.env.LOCALAPPDATA) {
    return join(process.env.LOCALAPPDATA, 'lnreadertui');
  }
  return join(homedir(), '.local', 'share', 'lnreadertui');
}

const dir = dataDir();
const libraryPath = join(dir, 'library.json');
if (!existsSync(libraryPath)) {
  console.log(`skip: no library.json at ${libraryPath}`);
  process.exit(0);
}
const library = JSON.parse(await readFile(libraryPath, 'utf8'));

/** Fixture stems (the novel id) that have reference output. */
const fixtureIds = new Set();
for (const name of await readdir(fixtures).catch(() => [])) {
  if (name.startsWith('reference-')) fixtureIds.add(name.slice('reference-'.length, -'.json'.length));
}

let failures = 0;
let checked = 0;
for (const book of library.books ?? []) {
  const file = join(dir, book.file);
  if (!existsSync(file)) {
    console.log(`skip ${book.title}: missing file`);
    continue;
  }
  const parsed = await readBookFile(file);
  const referenceId = String(book.sourceId ?? book.id);
  const referencePath = join(fixtures, `reference-${referenceId}.json`);
  if (fixtureIds.has(referenceId)) {
    const reference = JSON.parse(await readFile(referencePath, 'utf8'));
    failures += compare(book.title, parsed, reference);
  } else {
    checked += 1;
    const same = parsed.chapters.length === book.chapters;
    console.log(`${same ? 'ok  ' : 'FAIL'} ${book.title}: ${parsed.chapters.length} chapters (library.json says ${book.chapters})`);
    if (!same) failures += 1;
  }
}
if (checked === 0 && fixtureIds.size === 0) console.log('no fixtures and no library counts compared');

function compare(label, actual, reference) {
  const fail = (message) => {
    console.log(`FAIL ${label}: ${message}`);
    return 1;
  };
  if (actual.title !== reference.title) return fail(`title ${JSON.stringify(actual.title)} != ${JSON.stringify(reference.title)}`);
  if (actual.chapters.length !== reference.chapters.length) {
    return fail(`chapter count ${actual.chapters.length} != ${reference.chapters.length}`);
  }
  for (let i = 0; i < reference.chapters.length; i += 1) {
    const want = reference.chapters[i];
    const got = actual.chapters[i];
    if (got.title !== want.title) {
      return fail(`chapter ${i} title ${JSON.stringify(got.title)} != ${JSON.stringify(want.title)}`);
    }
    if (got.paragraphs.length !== want.paragraphs) {
      return fail(`chapter ${i} paragraph count ${got.paragraphs.length} != ${want.paragraphs}`);
    }
    const digest = createHash('sha256').update(JSON.stringify(got.paragraphs), 'utf8').digest('hex');
    if (digest !== want.sha256) {
      return fail(`chapter ${i} text differs (hash ${digest.slice(0, 12)} != ${want.sha256.slice(0, 12)})`);
    }
  }
  console.log(`ok   ${label}: ${actual.chapters.length} chapters, ${actual.chapters.reduce((n, c) => n + c.paragraphs.length, 0)} paragraphs`);
  return 0;
}

if (failures > 0) {
  console.error(`\n${failures} book(s) diverged from the terminal reader`);
  process.exit(1);
}
console.log('\nall books match the terminal reader');
