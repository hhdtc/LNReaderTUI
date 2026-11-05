/**
 * ZIP (EPUB container) reader and chapter extraction for the novel-reader
 * plugin.
 *
 * The chapter view is a faithful port of LNReaderTUI's `internal/epub/reader.go`
 * so that chapter indexes — the key a shared reading position is stored
 * under — stay identical between the terminal reader and this panel. Keep the
 * two in step: spine filtering, heading/title rules, paragraph flushing and
 * the fallback chapter title all change a chapter's identity.
 */

import { readFile } from 'node:fs/promises';
import { inflateRawSync } from 'node:zlib';

// ── ZIP container ───────────────────────────────────────────────────────────

const EOCD_SIG = 0x06054b50;
const CEN_SIG = 0x02014b50;
const LOC_SIG = 0x04034b50;

/**
 * Read the central directory of a ZIP archive held in memory.
 * @param {Buffer} buf - whole archive bytes.
 * @returns {Map<string, {method: number, compressedSize: number, size: number, offset: number}>}
 */
export function readZipIndex(buf) {
  const eocd = findEndOfCentralDirectory(buf);
  if (eocd < 0) throw new Error('not a ZIP archive: end of central directory not found');
  const entries = buf.readUInt16LE(eocd + 10);
  const directoryOffset = buf.readUInt32LE(eocd + 16);
  if (entries === 0xffff || directoryOffset === 0xffffffff) {
    throw new Error('ZIP64 archives are not supported');
  }
  const index = new Map();
  let at = directoryOffset;
  for (let i = 0; i < entries; i += 1) {
    if (at + 46 > buf.length || buf.readUInt32LE(at) !== CEN_SIG) break;
    const method = buf.readUInt16LE(at + 10);
    const compressedSize = buf.readUInt32LE(at + 20);
    const size = buf.readUInt32LE(at + 24);
    const nameLength = buf.readUInt16LE(at + 28);
    const extraLength = buf.readUInt16LE(at + 30);
    const commentLength = buf.readUInt16LE(at + 32);
    const offset = buf.readUInt32LE(at + 42);
    const name = buf.toString('utf8', at + 46, at + 46 + nameLength);
    index.set(name, { method, compressedSize, size, offset });
    at += 46 + nameLength + extraLength + commentLength;
  }
  return index;
}

function findEndOfCentralDirectory(buf) {
  const lowest = Math.max(0, buf.length - 0xffff - 22);
  for (let at = buf.length - 22; at >= lowest; at -= 1) {
    if (buf.readUInt32LE(at) === EOCD_SIG) return at;
  }
  return -1;
}

/**
 * Decompress one indexed entry.
 * @param {Buffer} buf - whole archive bytes.
 * @param {{method: number, compressedSize: number, size: number, offset: number}} entry
 * @returns {Buffer}
 */
export function readZipEntry(buf, entry) {
  if (buf.readUInt32LE(entry.offset) !== LOC_SIG) throw new Error('corrupt ZIP entry header');
  const nameLength = buf.readUInt16LE(entry.offset + 26);
  const extraLength = buf.readUInt16LE(entry.offset + 28);
  const start = entry.offset + 30 + nameLength + extraLength;
  const raw = buf.subarray(start, start + entry.compressedSize);
  if (entry.method === 0) return Buffer.from(raw);
  if (entry.method === 8) return inflateRawSync(raw);
  throw new Error(`unsupported ZIP compression method ${entry.method}`);
}

// ── Tolerant XHTML/XML tree ─────────────────────────────────────────────────

const VOID_ELEMENTS = new Set([
  'area', 'base', 'basefont', 'br', 'col', 'embed', 'frame', 'hr', 'img',
  'input', 'isindex', 'link', 'meta', 'param', 'source', 'track', 'wbr',
]);

const RAW_TEXT_ELEMENTS = new Set(['script', 'style']);

const NAMED_ENTITIES = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: '\u00a0',
  mdash: '\u2014', ndash: '\u2013', hellip: '\u2026', middot: '\u00b7',
  lsquo: '\u2018', rsquo: '\u2019', ldquo: '\u201c', rdquo: '\u201d',
  times: '\u00d7', divide: '\u00f7', copy: '\u00a9', reg: '\u00ae',
  deg: '\u00b0', plusmn: '\u00b1', laquo: '\u00ab', raquo: '\u00bb',
  bull: '\u2022', dagger: '\u2020', sect: '\u00a7', para: '\u00b6',
  prime: '\u2032', Prime: '\u2033', ensp: '\u2002', emsp: '\u2003',
  thinsp: '\u2009', shy: '\u00ad', zwj: '\u200d', zwnj: '\u200c',
};

/**
 * Decode character references in text or attribute values.
 * @param {string} text - raw source text.
 * @returns {string}
 */
export function decodeEntities(text) {
  if (!text.includes('&')) return text;
  return text.replace(/&(#x[0-9a-fA-F]+|#[0-9]+|[a-zA-Z][a-zA-Z0-9]*);/g, (whole, body) => {
    if (body[0] === '#') {
      const codePoint = body[1] === 'x' || body[1] === 'X'
        ? Number.parseInt(body.slice(2), 16)
        : Number.parseInt(body.slice(1), 10);
      if (!Number.isFinite(codePoint) || codePoint <= 0 || codePoint > 0x10ffff) return whole;
      try {
        return String.fromCodePoint(codePoint);
      } catch {
        return whole;
      }
    }
    return Object.prototype.hasOwnProperty.call(NAMED_ENTITIES, body) ? NAMED_ENTITIES[body] : whole;
  });
}

/**
 * Local name of a tag, lowercased, namespace prefix removed.
 * @param {string} tag
 * @returns {string}
 */
function localName(tag) {
  const colon = tag.indexOf(':');
  return (colon < 0 ? tag : tag.slice(colon + 1)).toLowerCase();
}

/**
 * Parse markup into a minimal DOM: element nodes carry `tag`, `attrs`, `children`;
 * text nodes are plain strings. Tolerant like the Go parser the terminal
 * reader uses: unknown end tags are ignored and unclosed elements nest.
 * @param {string} source - XHTML, HTML, or namespace-prefixed XML.
 * @returns {{type: 'document', children: any[]}}
 */
export function parseMarkup(source) {
  const document = { type: 'document', children: [] };
  /** @type {{tag: string, children: any[]}[]} */
  const stack = [{ tag: '#document', children: document.children }];
  const current = () => stack[stack.length - 1];
  const pushText = (value) => {
    if (value === '') return;
    current().children.push(value);
  };
  const indexOfOpen = (tag) => {
    for (let i = stack.length - 1; i > 0; i -= 1) if (stack[i].tag === tag) return i;
    return -1;
  };

  let at = 0;
  while (at < source.length) {
    const lt = source.indexOf('<', at);
    if (lt < 0) {
      pushText(decodeEntities(source.slice(at)));
      break;
    }
    if (lt > at) pushText(decodeEntities(source.slice(at, lt)));
    if (source.startsWith('<!--', lt)) {
      const end = source.indexOf('-->', lt + 4);
      at = end < 0 ? source.length : end + 3;
      continue;
    }
    if (source.startsWith('<![CDATA[', lt)) {
      const end = source.indexOf(']]>', lt + 9);
      at = end < 0 ? source.length : end + 3;
      continue;
    }
    if (source.startsWith('<!', lt) || source.startsWith('<?', lt)) {
      const end = source.indexOf('>', lt + 2);
      at = end < 0 ? source.length : end + 1;
      continue;
    }
    const gt = findTagEnd(source, lt + 1);
    if (gt < 0) {
      pushText(decodeEntities(source.slice(lt)));
      break;
    }
    const body = source.slice(lt + 1, gt);
    at = gt + 1;
    if (body[0] === '/') {
      const tag = localName(body.slice(1).trim());
      const open = indexOfOpen(tag);
      if (open > 0) stack.length = open;
      continue;
    }
    const selfClosing = body.endsWith('/');
    const inner = selfClosing ? body.slice(0, -1) : body;
    const nameMatch = /^[^\s/>]+/.exec(inner);
    if (nameMatch === null) continue;
    const rawTag = nameMatch[0];
    const tag = localName(rawTag);
    const node = { tag, attrs: parseAttributes(inner.slice(rawTag.length)), children: [] };
    current().children.push(node);
    if (selfClosing || VOID_ELEMENTS.has(tag)) continue;
    if (RAW_TEXT_ELEMENTS.has(tag)) {
      const close = source.toLowerCase().indexOf(`</${tag}`, at);
      const end = close < 0 ? source.length : close;
      node.children.push(source.slice(at, end));
      const after = close < 0 ? source.length : source.indexOf('>', close);
      at = after < 0 ? source.length : after + 1;
      continue;
    }
    stack.push(node);
  }
  return document;
}

function findTagEnd(source, from) {
  let quote = null;
  for (let i = from; i < source.length; i += 1) {
    const ch = source[i];
    if (quote !== null) {
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      continue;
    }
    if (ch === '>') return i;
  }
  return -1;
}

function parseAttributes(text) {
  const attrs = {};
  const pattern = /([^\s=/>]+)(?:\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+)))?/g;
  let match;
  while ((match = pattern.exec(text)) !== null) {
    const name = localName(match[1]);
    const raw = match[3] ?? match[4] ?? match[5] ?? '';
    attrs[name] = decodeEntities(raw);
  }
  return attrs;
}

/**
 * Every element with the given local name, depth first.
 * @param {any} node
 * @param {string} tag
 * @param {any[]} [out]
 * @returns {any[]}
 */
export function findAll(node, tag, out = []) {
  if (node.type === 'document') {
    for (const child of node.children) findAll(child, tag, out);
    return out;
  }
  if (typeof node !== 'object') return out;
  if (node.tag === tag) out.push(node);
  for (const child of node.children) findAll(child, tag, out);
  return out;
}

function textOf(node) {
  if (typeof node === 'string') return node;
  let out = '';
  for (const child of node.children ?? []) out += textOf(child);
  return out;
}

// ── Chapter extraction (parity with internal/epub/reader.go) ─────────────────

const BLOCK_ELEMENTS = new Set([
  'p', 'div', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'li', 'blockquote', 'pre',
  'section', 'article', 'table', 'tr', 'td', 'th', 'ul', 'ol', 'dl', 'figure',
  'figcaption', 'aside', 'header', 'footer', 'main', 'dd', 'dt', 'form',
  'fieldset', 'center', 'body', 'nav',
]);

/**
 * Split one XHTML document into its title and plain-text paragraphs.
 * @param {string} source
 * @param {string} name - archive path, for the fallback title.
 * @param {number} index - spine index, for the fallback title.
 * @returns {{title: string, paragraphs: string[]}}
 */
export function extractChapter(source, name, index) {
  const document = parseMarkup(source);
  const paragraphs = [];
  let buffer = '';
  let title = '';
  let foundTitle = false;

  const flush = () => {
    const text = buffer.trim();
    if (text !== '') paragraphs.push(text);
    buffer = '';
  };

  const walk = (node) => {
    if (typeof node === 'string') {
      buffer += node;
      return;
    }
    if (node.type === 'document') {
      for (const child of node.children) walk(child);
      return;
    }
    const tag = node.tag;
    if (tag === 'script' || tag === 'style' || tag === 'head' || tag === 'noscript') return;
    if (tag === 'br') {
      flush();
      return;
    }
    if (tag === 'img') {
      buffer += '[图]';
      return;
    }
    if (!foundTitle && (tag === 'h1' || tag === 'h2' || tag === 'h3')) {
      const heading = textOf(node).trim();
      if (heading !== '') {
        title = heading;
        foundTitle = true;
      }
      return;
    }
    if (BLOCK_ELEMENTS.has(tag)) {
      if (buffer.length > 0) flush();
      for (const child of node.children) walk(child);
      flush();
      return;
    }
    for (const child of node.children) walk(child);
  };

  walk(document);
  flush();
  if (title === '') title = fallbackChapterTitle(name, index);
  return { title, paragraphs };
}

function fallbackChapterTitle(name, index) {
  const base = name.slice(name.lastIndexOf('/') + 1);
  const dot = base.lastIndexOf('.');
  const stem = dot < 0 ? base : base.slice(0, dot);
  if (stem !== '' && stem !== 'ch') return stem;
  return `Chapter ${index + 1}`;
}

// ── EPUB / TXT ──────────────────────────────────────────────────────────────

/**
 * Parse a local book file.
 * @param {string} path - absolute path to a .epub or .txt file.
 * @returns {Promise<{title: string, author: string, chapters: {title: string, paragraphs: string[]}[]}>}
 */
export async function readBookFile(path) {
  const lower = path.toLowerCase();
  if (lower.endsWith('.epub')) return readEpub(path);
  if (lower.endsWith('.txt')) return readTxt(path);
  throw new Error(`unsupported file type: ${path.slice(path.lastIndexOf('.'))}`);
}

async function readEpub(path) {
  const buf = await readFile(path);
  const index = readZipIndex(buf);
  const read = (name) => {
    const entry = index.get(name);
    if (entry === undefined) throw new Error(`missing ${name}`);
    return readZipEntry(buf, entry).toString('utf8');
  };

  const containerDocument = parseMarkup(read('META-INF/container.xml'));
  const rootfile = findAll(containerDocument, 'rootfile')[0];
  const opfPath = rootfile?.attrs?.['full-path'];
  if (typeof opfPath !== 'string' || opfPath === '') {
    throw new Error('container.xml has no rootfile');
  }

  const opf = parseMarkup(read(opfPath));
  const metadata = findAll(opf, 'metadata')[0];
  const title = (findAll(metadata ?? opf, 'title')[0] !== undefined
    ? textOf(findAll(metadata ?? opf, 'title')[0]).trim()
    : '');
  const author = (findAll(metadata ?? opf, 'creator')[0] !== undefined
    ? textOf(findAll(metadata ?? opf, 'creator')[0]).trim()
    : '');

  const byId = new Map();
  const manifest = findAll(opf, 'manifest')[0];
  for (const item of findAll(manifest ?? opf, 'item')) {
    const attrs = item.attrs ?? {};
    const mediaType = String(attrs['media-type'] ?? '').toLowerCase();
    if (mediaType.includes('css') || mediaType.includes('image/')
      || mediaType.includes('ncx') || mediaType.includes('oebps-package')
      || mediaType.includes('font') || mediaType.includes('javascript')) continue;
    const href = String(attrs.href ?? '');
    const lowerHref = href.toLowerCase();
    if (lowerHref.endsWith('nav.xhtml') || lowerHref.endsWith('toc.ncx')) continue;
    const base = lowerHref.slice(lowerHref.lastIndexOf('/') + 1);
    const properties = String(attrs.properties ?? '');
    if (properties.includes('cover-image')
      || base === 'cover.xhtml' || base === 'cover.htm' || base === 'cover.html') continue;
    byId.set(attrs.id, href);
  }

  const baseDir = opfPath.slice(0, opfPath.lastIndexOf('/'));
  const chapters = [];
  const spine = findAll(opf, 'spine')[0];
  const refs = findAll(spine ?? opf, 'itemref');
  for (let i = 0; i < refs.length; i += 1) {
    const href = byId.get(refs[i].attrs?.idref);
    if (href === undefined) continue;
    const full = joinPath(baseDir, href);
    const entry = index.get(full);
    if (entry === undefined) continue;
    const chapter = extractChapter(readZipEntry(buf, entry).toString('utf8'), full, i);
    if (chapter.paragraphs.length > 0) chapters.push(chapter);
  }
  if (chapters.length === 0) throw new Error('epub has no readable chapters');
  return {
    title: title !== '' ? title : basenameWithoutExtension(path),
    author,
    chapters,
  };
}

function joinPath(dir, href) {
  const clean = href.split('#')[0].split('?')[0];
  return dir === '' ? clean : `${dir}/${clean}`;
}

function basenameWithoutExtension(path) {
  const base = path.slice(path.lastIndexOf('/') + 1);
  const dot = base.lastIndexOf('.');
  return dot < 0 ? base : base.slice(0, dot);
}

const TXT_CHAPTER = /^(?:第[一二三四五六七八九十百千\d]+[章話节節回]|Chapter\s*\d+|CHAPTER\s*\d+|第\d+章)/gm;

async function readTxt(path) {
  const text = (await readFile(path)).toString('utf8').replace(/\r\n/g, '\n');
  const title = basenameWithoutExtension(path);
  const chapters = [];
  const starts = [];
  TXT_CHAPTER.lastIndex = 0;
  let match;
  while ((match = TXT_CHAPTER.exec(text)) !== null) {
    starts.push({ start: match.index, marker: match[0] });
    if (match[0] === '') TXT_CHAPTER.lastIndex += 1;
  }
  if (starts.length >= 2) {
    for (let i = 0; i < starts.length; i += 1) {
      const from = starts[i].start;
      const to = i + 1 < starts.length ? starts[i + 1].start : text.length;
      const paragraphs = text.slice(from, to).trim().split('\n\n')
        .map((paragraph) => paragraph.trim())
        .filter((paragraph) => paragraph !== '');
      chapters.push({ title: starts[i].marker.trim(), paragraphs });
    }
    return { title, author: 'Unknown', chapters };
  }

  const runes = Array.from(text);
  const chunkSize = 3000;
  for (let i = 0; i < runes.length; i += chunkSize) {
    const chunk = runes.slice(i, i + chunkSize).join('').trim();
    if (chunk === '') continue;
    chapters.push({ title: `Page ${chapters.length + 1}`, paragraphs: chunk.split('\n') });
  }
  if (chapters.length === 0) throw new Error('txt has no readable content');
  return { title, author: 'Unknown', chapters };
}
