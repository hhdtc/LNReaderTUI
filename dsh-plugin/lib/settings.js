/**
 * Reader presentation settings.
 *
 * These are display preferences for this panel only — theme, font, size, ink,
 * and paper — so they live in their own file beside `library.json` instead of
 * touching the shared library schema the terminal reader owns. Writes are
 * atomic (temp file plus rename) and every value is clamped on the way in, so a
 * stale or hand-edited file cannot break rendering.
 */

import { randomBytes } from 'node:crypto';
import { readFile, rename, writeFile } from 'node:fs/promises';
import path from 'node:path';

/** Defaults used when no settings file exists yet. */
export const READER_DEFAULTS = Object.freeze({
  theme: 'nerv',
  font: 'serif',
  size: 17,
  ink: '#d8d8d0',
  paper: '#080807',
  margin: 36,
});

/** Font choices the client offers; the browser resolves each stack. */
export const READER_FONTS = Object.freeze(['serif', 'sans', 'kai', 'mono']);
/** Built-in reading themes; `custom` keeps the colours the client sends. */
export const READER_THEMES = Object.freeze(['nerv', 'paper', 'sepia', 'night', 'custom']);
/** Smallest and largest readable font size, in CSS pixels. */
export const READER_SIZE_RANGE = Object.freeze([12, 32]);
/** Smallest and largest page margin, in CSS pixels. */
export const READER_MARGIN_RANGE = Object.freeze([8, 120]);

const FILE = 'dsh-reader-settings.json';

/**
 * Clamp one candidate to a settings object with every field present.
 * @param {unknown} value
 * @returns {{theme: string, font: string, size: number, ink: string, paper: string}}
 */
export function normalizeSettings(value) {
  const source = value !== null && typeof value === 'object' ? value : {};
  const color = (candidate, fallback) => (
    typeof candidate === 'string' && /^#[0-9a-f]{6}$/i.test(candidate) ? candidate.toLowerCase() : fallback
  );
  const [min, max] = READER_SIZE_RANGE;
  const [marginMin, marginMax] = READER_MARGIN_RANGE;
  return {
    theme: READER_THEMES.includes(source.theme) ? source.theme : READER_DEFAULTS.theme,
    font: READER_FONTS.includes(source.font) ? source.font : READER_DEFAULTS.font,
    size: Number.isFinite(source.size) ? Math.min(max, Math.max(min, Math.round(source.size))) : READER_DEFAULTS.size,
    ink: color(source.ink, READER_DEFAULTS.ink),
    paper: color(source.paper, READER_DEFAULTS.paper),
    margin: Number.isFinite(source.margin)
      ? Math.min(marginMax, Math.max(marginMin, Math.round(source.margin)))
      : READER_DEFAULTS.margin,
  };
}

/**
 * Read the stored settings, falling back to the defaults.
 * @param {string} dataDir
 * @returns {Promise<{theme: string, font: string, size: number, ink: string, paper: string}>}
 */
export async function readSettings(dataDir) {
  try {
    return normalizeSettings(JSON.parse(await readFile(path.join(dataDir, FILE), 'utf8')));
  } catch (error) {
    if (error.code === 'ENOENT' || error instanceof SyntaxError) return { ...READER_DEFAULTS };
    throw error;
  }
}

/**
 * Merge a patch into the stored settings and persist the result.
 * @param {string} dataDir
 * @param {unknown} patch
 * @returns {Promise<{theme: string, font: string, size: number, ink: string, paper: string}>}
 */
export async function saveSettings(dataDir, patch) {
  const current = await readSettings(dataDir);
  const source = patch !== null && typeof patch === 'object' ? patch : {};
  const next = normalizeSettings({ ...current, ...source });
  const target = path.join(dataDir, FILE);
  const temporary = `${target}.tmp-${process.pid}-${randomBytes(4).toString('hex')}`;
  await writeFile(temporary, `${JSON.stringify(next, null, 2)}\n`, 'utf8');
  await rename(temporary, target);
  return next;
}
