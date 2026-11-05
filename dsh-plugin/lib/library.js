/**
 * The LNReaderTUI library store, read and written in place.
 *
 * This module deliberately speaks the terminal reader's on-disk format —
 * `<dataDir>/library.json` plus `<dataDir>/books/` — so the panel and the TUI
 * share one library and one reading position. Field names, the atomic
 * write-and-rename, and the progress record shape are copies of
 * `internal/model/library.go`; keep them in step.
 */

import { mkdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { homedir } from 'node:os';
import { join } from 'node:path';

/** Progress record as the TUI stores it. */
export const EMPTY_PROGRESS = { chapterIndex: 0, offset: 0, lastReadAt: '' };

/**
 * Resolve the library directory the terminal reader would use:
 * `$XDG_DATA_HOME/lnreadertui`, `%LOCALAPPDATA%\lnreadertui` on Windows, else
 * `~/.local/share/lnreadertui`.
 * @param {string | undefined} override - explicit plugin configuration.
 * @returns {string}
 */
export function resolveDataDir(override) {
  if (typeof override === 'string' && override.trim() !== '') return override;
  const fromEnv = process.env.LNREADERTUI_DATA_DIR;
  if (typeof fromEnv === 'string' && fromEnv.trim() !== '') return fromEnv;
  if (process.env.XDG_DATA_HOME) return join(process.env.XDG_DATA_HOME, 'lnreadertui');
  if (process.platform === 'win32' && process.env.LOCALAPPDATA) {
    return join(process.env.LOCALAPPDATA, 'lnreadertui');
  }
  return join(homedir(), '.local', 'share', 'lnreadertui');
}

/**
 * Create the library directory (and its `books/` companion) when missing.
 * @param {string} dataDir
 */
export async function ensureDataDir(dataDir) {
  await mkdir(join(dataDir, 'books'), { recursive: true });
}

/** Serialized read-modify-write queue so panel writes never interleave. */
let tail = Promise.resolve();

function serialize(work) {
  const run = tail.then(work, work);
  tail = run.then(() => undefined, () => undefined);
  return run;
}

/**
 * Read the raw library index.
 * @param {string} dataDir
 * @returns {Promise<{books: any[]}>}
 */
export async function readLibrary(dataDir) {
  const path = join(dataDir, 'library.json');
  let text;
  try {
    text = await readFile(path, 'utf8');
  } catch (error) {
    if (error && error.code === 'ENOENT') return { books: [] };
    throw error;
  }
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (error) {
    throw new Error(`library.json: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (parsed === null || typeof parsed !== 'object' || !Array.isArray(parsed.books)) {
    throw new Error('library.json: unexpected shape');
  }
  return parsed;
}

/**
 * Write the library index atomically (temporary sibling, then rename), matching
 * the TUI's own save.
 * @param {string} dataDir
 * @param {{books: any[]}} library
 */
async function writeLibrary(dataDir, library) {
  await ensureDataDir(dataDir);
  const path = join(dataDir, 'library.json');
  const temporary = `${path}.tmp-${process.pid}-${randomBytes(4).toString('hex')}`;
  const text = `${JSON.stringify({ books: library.books }, null, 2)}\n`;
  await writeFile(temporary, text, 'utf8');
  await rename(temporary, path);
}

/**
 * The progress line the terminal reader's library list shows.
 * @param {any} progress
 * @returns {{read: boolean, label: string}}
 */
export function describeProgress(progress) {
  const value = progress ?? EMPTY_PROGRESS;
  const read = !(value.lastReadAt === '' && value.chapterIndex === 0);
  if (!read) return { read: false, label: 'unread' };
  const percent = Math.round((Number(value.offset) || 0) * 100);
  const title = typeof value.chapterTitle === 'string' ? value.chapterTitle : '';
  return { read: true, label: title === '' ? `${percent}%` : `${title} ${percent}%` };
}

/**
 * Library rows for the panel: the stored record plus facts the file system can
 * answer (present file, byte size), sorted like the TUI: most recently read
 * first, then by title.
 * @param {string} dataDir
 * @returns {Promise<any[]>}
 */
export async function listBooks(dataDir) {
  const library = await readLibrary(dataDir);
  const rows = [];
  for (const book of library.books) {
    if (book === null || typeof book !== 'object') continue;
    const path = join(dataDir, String(book.file ?? ''));
    let present = false;
    let sizeBytes = null;
    try {
      const info = await stat(path);
      present = info.isFile();
      sizeBytes = info.size;
    } catch {
      present = false;
    }
    rows.push({ ...book, present, sizeBytes, summary: describeProgress(book.progress) });
  }
  rows.sort((left, right) => {
    const leftAt = left.progress?.lastReadAt ?? '';
    const rightAt = right.progress?.lastReadAt ?? '';
    if (leftAt !== rightAt) return leftAt < rightAt ? 1 : -1;
    return String(left.title ?? '').localeCompare(String(right.title ?? ''));
  });
  return rows;
}

/**
 * Find one book by id.
 * @param {string} dataDir
 * @param {string} id
 * @returns {Promise<any | null>}
 */
export async function findBook(dataDir, id) {
  const library = await readLibrary(dataDir);
  return library.books.find((book) => book !== null && typeof book === 'object' && book.id === id) ?? null;
}

/** RFC 3339 seconds precision, as `time.RFC3339` writes it. */
function rfc3339(date) {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/**
 * Persist a reading position for one book, keeping every other field intact.
 * Mirrors `Store.SetProgress`: negative indexes clamp to 0, the offset clamps
 * to 0..1, and an absent chapter title is omitted.
 * @param {string} dataDir
 * @param {string} id
 * @param {{chapterIndex: number, offset: number, chapterTitle?: string}} position
 * @returns {Promise<any>} the stored progress record.
 */
export function setProgress(dataDir, id, position) {
  return serialize(async () => {
    const library = await readLibrary(dataDir);
    const book = library.books.find((entry) => entry?.id === id);
    if (book === undefined) throw new Error(`book not found: ${id}`);
    let chapterIndex = Math.trunc(Number(position.chapterIndex) || 0);
    if (chapterIndex < 0) chapterIndex = 0;
    let offset = Number(position.offset) || 0;
    if (!Number.isFinite(offset)) offset = 0;
    if (offset < 0) offset = 0;
    if (offset > 1) offset = 1;
    const chapterTitle = typeof position.chapterTitle === 'string' ? position.chapterTitle : '';
    const progress = {
      chapterIndex,
      offset,
      ...(chapterTitle === '' ? {} : { chapterTitle }),
      lastReadAt: rfc3339(new Date()),
    };
    book.progress = progress;
    await writeLibrary(dataDir, library);
    return progress;
  });
}

/**
 * Clear one book's reading position, as `books reset` does.
 * @param {string} dataDir
 * @param {string} id
 * @returns {Promise<{id: string}>}
 */
export function resetProgress(dataDir, id) {
  return serialize(async () => {
    const library = await readLibrary(dataDir);
    const book = library.books.find((entry) => entry?.id === id);
    if (book === undefined) throw new Error(`book not found: ${id}`);
    book.progress = { ...EMPTY_PROGRESS };
    await writeLibrary(dataDir, library);
    return { id };
  });
}

/**
 * Remove one book and its file, as `books delete` does.
 * @param {string} dataDir
 * @param {string} id
 * @returns {Promise<{id: string, file: string | null}>}
 */
export function deleteBook(dataDir, id) {
  return serialize(async () => {
    const library = await readLibrary(dataDir);
    const index = library.books.findIndex((entry) => entry?.id === id);
    if (index < 0) return { id, file: null };
    const [removed] = library.books.splice(index, 1);
    await writeLibrary(dataDir, library);
    if (typeof removed.file === 'string' && removed.file !== '') {
      await rm(join(dataDir, removed.file), { force: true });
    }
    return { id, file: removed.file ?? null };
  });
}

/**
 * Absolute path of a book file.
 * @param {string} dataDir
 * @param {any} book
 * @returns {string}
 */
export function bookFilePath(dataDir, book) {
  return join(dataDir, String(book.file ?? ''));
}
