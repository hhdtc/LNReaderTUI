/**
 * DSH Web client half of the novel reader.
 *
 * Registers one keyed `main` panel plus its left-sidebar launcher, so the panel
 * is reached exactly like the built-in pages (Plugins, Schedule, …). All data
 * comes from this package's host half over `/novel-reader/api`; the panel never
 * touches the filesystem itself.
 */

window.__ModuleLoader__.load({
  id: 'dsh-novel-reader',
  factory: (require) => {
    const React = require('react');
    const h = React.createElement;

    const PANEL_ID = 'novel-reader';
    const API_BASE = '/novel-reader/api';
    const PAGE_STEP_RATIO = 0.92;

    // ── copy ────────────────────────────────────────────────────────────────

    const zh = {
      panel: '小说',
      title: '小说阅读',
      tabLibrary: '书库',
      tabSearch: '搜索',
      search: '搜索',
      booksCount: '{n} 本',
      refresh: '刷新',
      loading: '加载中…',
      emptyLibrary: '书库还是空的：搜索一本小说下载，或导入本地 EPUB / TXT。',
      emptySearch: '输入书名、作者或 bilinovel 小说 ID / 链接',
      noResults: '没有找到结果。可以直接填小说 ID（如 4649）或小说链接。',
      searchHint: '输入即搜；回车立即搜索。',
      searching: '正在搜索…',
      searchFailed: '搜索失败',
      detailFailed: '获取详情失败',
      detail: '详情',
      download: '下载',
      downloading: '下载中',
      update: '更新',
      updating: '更新中',
      read: '阅读',
      open: '打开',
      back: '返回',
      backToLibrary: '返回书库',
      cancel: '取消',
      cancelJob: '取消任务',
      reset: '重置进度',
      delete: '删除',
      remove: '移除',
      confirmReset: '重置这本书的阅读进度？',
      confirmDelete: '从书库删除这本书？文件也会一并删除。',
      author: '作者',
      status: '状态',
      tags: '标签',
      publisher: '出版社',
      rating: '评分',
      chaptersCount: '{n} 章',
      volumesCount: '{n} 卷',
      introduction: '简介',
      catalog: '目录',
      chapters: '章节',
      unread: '未读',
      chapterProgress: '第 {index}/{total} 章',
      inLibrary: '已在书库',
      notInLibrary: '未下载',
      imported: '已导入 {n} 本',
      importPath: '导入路径',
      importAction: '导入',
      importPlaceholder: '本地 .epub / .txt 文件或目录的绝对路径',
      importHint: '等价于 lnreadertui import <路径>',
      jobDone: '已完成',
      jobFailed: '失败',
      jobCancelled: '已取消',
      jobRunning: '进行中',
      savedTo: '已保存到 {path}',
      addedChapters: '新增 {n} 章，共 {total} 章',
      cliMissing: '未找到 lnreadertui 可执行文件：安装 LNReaderTUI，或把路径设到环境变量 LNREADERTUI_BIN。',
      dataDir: '数据目录',
      dataDirMissing: '读取数据目录失败',
      libraryError: '书库读取失败',
      prevChapter: '上一章',
      nextChapter: '下一章',
      chapterPanel: '目录',
      pageHint: '← / → 翻页 · n / p 切章 · s 目录 · Esc 返回',
      position: '第 {index}/{total} 章 · 本章 {percent}%',
      pageOf: '第 {page}/{pages} 页',
      prevPage: '上一页',
      nextPage: '下一页',
      pageHint: '← / → 翻页 · n / p 切章 · s 目录 · Esc 返回',
      loadingChapter: '载入中…',
      bookMissing: '文件缺失',
      filterPlaceholder: '过滤书库…',
      logs: '日志',
      showLogs: '查看日志',
      hideLogs: '收起日志',
      unknownAuthor: '未知作者',
      readerEnd: '本章完',
    };
    const en = {
      panel: 'Novels',
      title: 'Novel Reader',
      tabLibrary: 'Library',
      tabSearch: 'Search',
      search: 'Search',
      booksCount: '{n} books',
      refresh: 'Refresh',
      loading: 'Loading…',
      emptyLibrary: 'The library is empty: download a novel or import a local EPUB / TXT.',
      emptySearch: 'Book title, author, bilinovel novel id or URL',
      noResults: 'No results. A novel id (e.g. 4649) or a novel URL works directly.',
      searchHint: 'Search as you type; Enter searches immediately.',
      searching: 'Searching…',
      searchFailed: 'Search failed',
      detailFailed: 'Could not load the detail page',
      detail: 'Detail',
      download: 'Download',
      downloading: 'Downloading',
      update: 'Update',
      updating: 'Updating',
      read: 'Read',
      open: 'Open',
      back: 'Back',
      backToLibrary: 'Back to library',
      cancel: 'Cancel',
      cancelJob: 'Cancel job',
      reset: 'Reset progress',
      delete: 'Delete',
      remove: 'Remove',
      confirmReset: 'Reset the reading position of this book?',
      confirmDelete: 'Delete this book from the library? Its file is deleted too.',
      author: 'Author',
      status: 'Status',
      tags: 'Tags',
      publisher: 'Publisher',
      rating: 'Rating',
      chaptersCount: '{n} chapters',
      volumesCount: '{n} volumes',
      introduction: 'Synopsis',
      catalog: 'Catalog',
      chapters: 'Chapters',
      unread: 'Unread',
      chapterProgress: 'Chapter {index}/{total}',
      inLibrary: 'In library',
      notInLibrary: 'Not downloaded',
      imported: 'Imported {n} book(s)',
      importPath: 'Import path',
      importAction: 'Import',
      importPlaceholder: 'Absolute path to a local .epub / .txt file or directory',
      importHint: 'Same as lnreadertui import <path>',
      jobDone: 'Done',
      jobFailed: 'Failed',
      jobCancelled: 'Cancelled',
      jobRunning: 'Running',
      savedTo: 'Saved to {path}',
      addedChapters: 'Added {n} chapters, {total} total',
      cliMissing: 'The lnreadertui executable was not found. Install LNReaderTUI or set LNREADERTUI_BIN.',
      dataDir: 'Data directory',
      dataDirMissing: 'Could not read the data directory',
      libraryError: 'Could not read the library',
      prevChapter: 'Previous chapter',
      nextChapter: 'Next chapter',
      chapterPanel: 'Catalog',
      pageHint: '← / → page · n / p chapter · s catalog · Esc back',
      position: 'Chapter {index}/{total} · {percent}% of chapter',
      pageOf: 'Page {page}/{pages}',
      prevPage: 'Previous page',
      nextPage: 'Next page',
      pageHint: '← / → page · n / p chapter · s catalog · Esc back',
      loadingChapter: 'Loading…',
      bookMissing: 'File missing',
      filterPlaceholder: 'Filter the library…',
      logs: 'Logs',
      showLogs: 'Show logs',
      hideLogs: 'Hide logs',
      unknownAuthor: 'Unknown author',
      readerEnd: 'End of chapter',
    };

    const language = (typeof navigator !== 'undefined' && String(navigator.language || '').toLowerCase().startsWith('zh'))
      ? 'zh'
      : 'en';
    const dict = language === 'zh' ? zh : en;

    /** Translate one key with `{name}` interpolation. */
    function t(key, params) {
      const template = dict[key] ?? zh[key] ?? key;
      return params === undefined ? template : template.replace(/\{(\w+)\}/g, (whole, name) => (name in params ? String(params[name]) : whole));
    }

    // ── transport ───────────────────────────────────────────────────────────

    /** One API failure carrying the host's error code. */
    class ApiError extends Error {
      constructor(code, message) {
        super(message);
        this.code = code;
      }
    }

    async function request(path, init) {
      let response;
      try {
        response = await fetch(`${API_BASE}${path}`, {
          ...(init ?? {}),
          headers: { 'content-type': 'application/json', ...(init?.headers ?? {}) },
          credentials: 'same-origin',
          cache: 'no-store',
        });
      } catch (error) {
        throw new ApiError('NETWORK', error instanceof Error ? error.message : String(error));
      }
      let payload = null;
      try {
        payload = await response.json();
      } catch {
        payload = null;
      }
      if (!response.ok) {
        const detail = payload?.error ?? {};
        throw new ApiError(detail.code ?? `HTTP_${response.status}`, detail.message ?? response.statusText);
      }
      return payload;
    }

    const api = {
      status: () => request('/status'),
      books: () => request('/books'),
      chapters: (id) => request(`/books/${encodeURIComponent(id)}/chapters`),
      chapter: (id, index) => request(`/books/${encodeURIComponent(id)}/chapter/${index}`),
      saveProgress: (id, position) => request(`/books/${encodeURIComponent(id)}/progress`, {
        method: 'POST',
        body: JSON.stringify(position),
        keepalive: true,
      }),
      resetProgress: (id) => request(`/books/${encodeURIComponent(id)}/reset`, { method: 'POST', body: '{}' }),
      deleteBook: (id) => request(`/books/${encodeURIComponent(id)}/delete`, { method: 'POST', body: '{}' }),
      importPath: (path) => request('/import', { method: 'POST', body: JSON.stringify({ path }) }),
      search: (query) => request(`/search?q=${encodeURIComponent(query)}`),
      detail: (query) => request(`/detail?q=${encodeURIComponent(query)}`),
      download: (query) => request('/download', { method: 'POST', body: JSON.stringify({ query }) }),
      update: (query) => request('/update', { method: 'POST', body: JSON.stringify({ query }) }),
      /** URL of one illustration read out of a book archive. */
      imageUrl: (id, entry) => `${API_BASE}/books/${encodeURIComponent(id)}/image?path=${encodeURIComponent(entry)}`,
      /** URL of a book's own cover image. */
      coverUrl: (id) => `${API_BASE}/books/${encodeURIComponent(id)}/cover`,
      job: (id) => request(`/jobs/${encodeURIComponent(id)}`),
      cancelJob: (id) => request(`/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: '{}' }),
    };

    // ── styles ──────────────────────────────────────────────────────────────

    const CSS = `
      .nr-page{box-sizing:border-box;height:100%;display:flex;flex-direction:column;color:var(--dsw-alias-label-primary);
        font-size:14px;line-height:1.5;--nr-inset:clamp(16px,3vw,32px)}
      .nr-head{flex:none;display:flex;flex-direction:column;gap:10px;padding:calc(20px + var(--dsh-frame-top-clearance,0px)) var(--nr-inset) 10px;
        border-bottom:.5px solid var(--dsw-alias-border-l3);background:var(--dsw-alias-bg-base);position:relative;z-index:2}
      .nr-title-row{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}
      .nr-title{margin:0;font-size:20px;font-weight:500;line-height:28px}
      .nr-subtle{color:var(--dsw-alias-label-tertiary);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:60ch}
      .nr-tabs{display:flex;gap:4px;align-items:center;flex-wrap:wrap}
      .nr-tab{border:0;background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;padding:5px 12px;border-radius:var(--dsw-radius-sm,#6px);cursor:pointer}
      .nr-tab:hover{background:var(--dsw-alias-interactive-bg-hover,rgba(127,127,127,.12));color:var(--dsw-alias-label-primary)}
      .nr-tab[data-active="true"]{background:var(--dsw-alias-bg-layer-3,rgba(127,127,127,.16));color:var(--dsw-alias-label-primary);font-weight:500}
      .nr-spacer{flex:1}
      .nr-body{flex:1;min-height:0;overflow:auto;padding:16px var(--nr-inset) 32px;scrollbar-gutter:stable}
      .nr-body--flush{padding:0;overflow:hidden;display:flex;flex-direction:column}
      .nr-btn{border:.5px solid var(--dsw-alias-border-l3);background:var(--dsw-alias-bg-layer-1,transparent);color:var(--dsw-alias-label-primary);
        font:inherit;font-size:13px;padding:5px 10px;border-radius:var(--dsw-radius-sm,#6px);cursor:pointer;white-space:nowrap}
      .nr-btn:hover{background:var(--dsw-alias-interactive-bg-hover,rgba(127,127,127,.12))}
      .nr-btn:disabled{opacity:.5;cursor:default}
      .nr-btn--primary{border-color:transparent;background:var(--dsw-alias-state-business-primary,#2f6fed);color:#fff}
      .nr-btn--primary:hover{filter:brightness(1.06);background:var(--dsw-alias-state-business-primary,#2f6fed)}
      .nr-btn--danger{color:var(--dsw-alias-state-error-primary,#e5484d)}
      .nr-btn--ghost{border-color:transparent;background:transparent;color:var(--dsw-alias-label-secondary)}
      .nr-input{box-sizing:border-box;flex:1;min-width:0;border:.5px solid var(--dsw-alias-border-l3);background:var(--dsw-alias-bg-layer-1,transparent);
        color:inherit;font:inherit;padding:7px 10px;border-radius:var(--dsw-radius-sm,#6px);outline:none}
      .nr-input:focus{border-color:var(--dsw-focus-ring-color,#2f6fed)}
      .nr-row{display:flex;gap:8px;align-items:center}
      .nr-chip{display:inline-flex;align-items:center;gap:4px;border-radius:999px;padding:1px 8px;font-size:12px;
        background:var(--dsw-alias-bg-layer-3,rgba(127,127,127,.14));color:var(--dsw-alias-label-secondary)}
      .nr-chip--ok{color:var(--dsw-alias-state-success-primary,#1a7f37);background:rgba(26,127,55,.12)}
      .nr-chip--warn{color:var(--dsw-alias-state-warn-primary,#b26a00);background:rgba(178,106,0,.12)}
      .nr-chip--bad{color:var(--dsw-alias-state-error-primary,#e5484d);background:rgba(229,72,77,.12)}
      .nr-list{display:flex;flex-direction:column;gap:6px;margin:0;padding:0;list-style:none}
      .nr-card{box-sizing:border-box;width:100%;text-align:left;border:.5px solid var(--dsw-alias-border-l3);background:var(--dsw-alias-bg-layer-1,transparent);
        border-radius:var(--dsw-radius-md,10px);padding:10px 12px;display:flex;gap:12px;align-items:center;cursor:pointer;color:inherit;font:inherit}
      .nr-card:hover{background:var(--dsw-alias-interactive-bg-hover,rgba(127,127,127,.1))}
      .nr-cover{flex:none;width:34px;height:46px;border-radius:5px;display:grid;place-items:center;font-weight:600;font-size:16px;
        background:linear-gradient(160deg,rgba(47,111,237,.22),rgba(47,111,237,.06));color:var(--dsw-alias-label-primary);overflow:hidden}
      .nr-cover img{width:100%;height:100%;object-fit:cover}
      .nr-card-main{flex:1;min-width:0;display:flex;flex-direction:column;gap:3px}
      .nr-card-title{font-weight:500;font-size:14px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
      .nr-card-meta{display:flex;gap:8px;align-items:center;font-size:12px;color:var(--dsw-alias-label-tertiary);flex-wrap:wrap}
      .nr-card-actions{flex:none;display:flex;gap:6px}
      .nr-bar{position:relative;height:3px;border-radius:2px;background:var(--dsw-alias-bg-layer-3,rgba(127,127,127,.2));overflow:hidden;margin-top:4px}
      .nr-bar > i{position:absolute;inset:0 auto 0 0;background:var(--dsw-alias-state-business-primary,#2f6fed);border-radius:2px}
      .nr-panel{border:.5px solid var(--dsw-alias-border-l3);border-radius:var(--dsw-radius-md,10px);padding:12px 14px;background:var(--dsw-alias-bg-layer-1,transparent);
        display:flex;flex-direction:column;gap:8px;margin-top:12px}
      .nr-panel-title{font-weight:500;font-size:13px;color:var(--dsw-alias-label-secondary);display:flex;gap:8px;align-items:center}
      .nr-job-logs{margin:0;padding:8px 10px;font-family:var(--ds-font-family-code,monospace);font-size:11.5px;line-height:1.45;
        max-height:180px;overflow:auto;background:var(--dsw-alias-bg-layer-3,rgba(127,127,127,.1));border-radius:6px;color:var(--dsw-alias-label-secondary);
        white-space:pre-wrap;word-break:break-word}
      .nr-error{color:var(--dsw-alias-state-error-primary,#e5484d);font-size:13px;white-space:pre-wrap}
      .nr-empty{color:var(--dsw-alias-label-tertiary);font-size:13px;padding:24px 0;text-align:center}
      .nr-search{display:flex;gap:8px;align-items:center;position:sticky;top:0;z-index:1;background:var(--dsw-alias-bg-base);padding-bottom:10px}
      .nr-section{margin:0 0 8px;font-size:13px;font-weight:500;color:var(--dsw-alias-label-secondary)}
      .nr-desc{white-space:pre-wrap;font-size:13.5px;line-height:1.75;color:var(--dsw-alias-label-primary)}
      .nr-detail-head{display:flex;gap:14px;align-items:flex-start;flex-wrap:wrap}
      .nr-detail-head .nr-cover{width:74px;height:100px;font-size:26px;border-radius:8px}
      .nr-detail-meta{display:flex;flex-direction:column;gap:4px;flex:1;min-width:220px}
      .nr-volumes{display:flex;flex-direction:column;gap:4px;margin-top:6px}
      .nr-volume{border:.5px solid var(--dsw-alias-border-l3);border-radius:var(--dsw-radius-sm,#6px);padding:6px 10px;background:var(--dsw-alias-bg-layer-1,transparent)}
      .nr-volume summary{cursor:pointer;font-size:13px;display:flex;gap:8px;align-items:center}
      .nr-volume ol{margin:6px 0 0;padding-inline-start:20px;font-size:12.5px;color:var(--dsw-alias-label-secondary);max-height:220px;overflow:auto}
      .nr-reader{display:flex;flex-direction:column;height:100%;min-height:0;background:var(--dsw-alias-bg-base)}
      .nr-reader-head{flex:none;display:flex;gap:8px;align-items:center;padding:calc(16px + var(--dsh-frame-top-clearance,0px)) var(--nr-inset) 10px;
        border-bottom:.5px solid var(--dsw-alias-border-l3);flex-wrap:wrap}
      .nr-reader-title{display:flex;flex-direction:column;gap:1px;min-width:0;flex:1}
      .nr-reader-book{font-weight:500;font-size:13.5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
      .nr-reader-chapter{font-size:12px;color:var(--dsw-alias-label-tertiary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
      .nr-reader-stage{flex:1;min-height:0;position:relative;display:flex}
      /* Page mode: one column per page, so a flip never splits a text line and
         the reading area never scrolls. */
      .nr-reader-viewport{flex:1;min-width:0;position:relative;overflow:hidden;cursor:pointer;user-select:text}
      /* One page wide and clipped: the neighbouring columns never bleed in. */
      .nr-page-clip{position:absolute;overflow:hidden}
      .nr-reader-flow{position:absolute;column-fill:auto;will-change:transform}
      .nr-article{font-size:16px;line-height:1.95;letter-spacing:.01em}
      .nr-article p{margin:0 0 1em;text-indent:2em;orphans:2;widows:2}
      .nr-article p:first-child{margin-top:0}
      .nr-article .nr-reader-end{text-align:center;text-indent:0;color:var(--dsw-alias-label-tertiary);margin-top:1.2em}
      /* Illustrations: a figure owns its line box and never exceeds one page,
         so a tall image cannot push a half line past the page edge. */
      .nr-article figure{margin:1.1em 0;text-align:center;text-indent:0}
      .nr-article figure img{display:block;max-width:100%;max-height:calc(var(--nr-page-height,540px) - 2.6em);
        height:auto;margin:0 auto;border-radius:4px;object-fit:contain}
      .nr-page-hint{position:absolute;inset-inline-end:14px;bottom:8px;font-size:11px;color:var(--dsw-alias-label-tertiary);
        background:var(--dsw-alias-bg-base);padding:2px 6px;border-radius:6px;pointer-events:none;opacity:.85}
      .nr-page-hint[data-few] { opacity:0 }
      .nr-chapters{width:min(320px,42%);flex:none;border-inline-start:.5px solid var(--dsw-alias-border-l3);overflow:auto;
        display:flex;flex-direction:column;background:var(--dsw-alias-bg-base)}
      .nr-chapters-head{position:sticky;top:0;background:var(--dsw-alias-bg-base);padding:10px 12px;border-bottom:.5px solid var(--dsw-alias-border-l3);
        font-size:12px;color:var(--dsw-alias-label-secondary);display:flex;justify-content:space-between;align-items:center;gap:8px}
      .nr-chapter{display:flex;gap:8px;align-items:baseline;width:100%;text-align:left;border:0;background:transparent;color:inherit;
        font:inherit;font-size:13px;padding:7px 12px;cursor:pointer}
      .nr-chapter:hover{background:var(--dsw-alias-interactive-bg-hover,rgba(127,127,127,.1))}
      .nr-chapter[data-current="true"]{background:var(--dsw-alias-bg-layer-3,rgba(127,127,127,.16));font-weight:500}
      .nr-chapter-index{color:var(--dsw-alias-label-tertiary);font-size:11.5px;font-variant-numeric:tabular-nums;min-width:2.4em}
      .nr-reader-foot{flex:none;display:flex;gap:10px;align-items:center;padding:6px var(--nr-inset);border-top:.5px solid var(--dsw-alias-border-l3);
        font-size:11.5px;color:var(--dsw-alias-label-tertiary);flex-wrap:wrap}
      .nr-toast{position:fixed;inset-inline-end:24px;bottom:24px;max-width:min(420px,80vw);background:var(--dsw-alias-bg-layer-3,#222);
        color:var(--dsw-alias-label-primary);border:.5px solid var(--dsw-alias-border-l3);border-radius:var(--dsw-radius-md,10px);
        padding:10px 14px;font-size:13px;box-shadow:0 8px 24px rgba(0,0,0,.24);z-index:40;display:flex;gap:10px;align-items:flex-start}
      .nr-toast button{margin-inline-start:auto}
      .nr-spinner{width:13px;height:13px;border-radius:50%;border:1.6px solid var(--dsw-alias-border-l3);
        border-top-color:var(--dsw-alias-state-business-primary,#2f6fed);display:inline-block;animation:nr-spin .8s linear infinite}
      @keyframes nr-spin{to{transform:rotate(360deg)}}
      @media (max-width:720px){.nr-card-actions{display:none}.nr-reader-title{flex-basis:100%;order:-1}}
    `;

    /** Inject the stylesheet once per plugin load. */
    function installStyles() {
      const element = document.createElement('style');
      element.dataset.novelReader = 'styles';
      element.textContent = CSS;
      document.head.appendChild(element);
      return () => element.remove();
    }

    // ── small parts ─────────────────────────────────────────────────────────

    function Icon({ size = 20 }) {
      return h('svg', {
        width: size,
        height: size,
        viewBox: '0 0 20 20',
        fill: 'none',
        'aria-hidden': 'true',
        stroke: 'currentColor',
        strokeWidth: 1.4,
        strokeLinecap: 'round',
        strokeLinejoin: 'round',
      }, [
        h('path', { key: 'spine', d: 'M4.2 3.4h4.1c.9 0 1.6.7 1.6 1.6v11.6c0-.9-.7-1.6-1.6-1.6H4.2c-.4 0-.7-.3-.7-.7V4.1c0-.4.3-.7.7-.7Z' }),
        h('path', { key: 'spine2', d: 'M15.8 3.4h-4.1c-.9 0-1.6.7-1.6 1.6v11.6c0-.9.7-1.6 1.6-1.6h4.1c.4 0 .7-.3.7-.7V4.1c0-.4-.3-.7-.7-.7Z' }),
        h('path', { key: 'lines', d: 'M12.6 7.1h1.9M12.6 9.6h1.9M5.6 7.1h1.9' }),
      ]);
    }

    function Spinner() {
      return h('span', { className: 'nr-spinner', 'aria-hidden': 'true' });
    }

    function ProgressBar({ fraction }) {
      const percent = Math.max(0, Math.min(1, Number(fraction) || 0)) * 100;
      return h('div', { className: 'nr-bar' }, h('i', { style: { width: `${percent}%` } }));
    }

    function Cover({ book }) {
      const title = book?.title ?? '';
      const cover = book?.coverUrl ?? '';
      const [failed, setFailed] = React.useState(false);
      const initial = Array.from(title)[0] ?? '书';
      if (cover === '' || failed) {
        return h('div', { className: 'nr-cover', 'aria-hidden': 'true' }, initial);
      }
      return h('div', { className: 'nr-cover' }, h('img', {
        src: cover,
        alt: '',
        loading: 'lazy',
        referrerPolicy: 'no-referrer',
        onError: () => setFailed(true),
      }));
    }

    function ErrorLine({ error, onDismiss }) {
      if (!error) return null;
      return h('div', { className: 'nr-panel' }, [
        h('div', { className: 'nr-error', key: 'message' }, error),
        onDismiss === undefined ? null : h('div', { key: 'actions' }, h('button', {
          className: 'nr-btn nr-btn--ghost',
          onClick: onDismiss,
          type: 'button',
        }, t('cancel'))),
      ]);
    }

    /** Reading position as a 0..1 fraction of the whole book. */
    function bookFraction(book) {
      const total = Math.max(1, Number(book?.chapters) || 1);
      const index = Math.max(0, Math.min(total - 1, Number(book?.progress?.chapterIndex) || 0));
      const offset = Math.max(0, Math.min(1, Number(book?.progress?.offset) || 0));
      return (index + offset) / total;
    }

    function progressText(book) {
      const progress = book?.progress;
      const read = !(progress?.lastReadAt === '' || progress?.lastReadAt === undefined) || (Number(progress?.chapterIndex) || 0) > 0;
      if (!read) return t('unread');
      const total = Number(book?.chapters) || 0;
      const index = (Number(progress?.chapterIndex) || 0) + 1;
      const percent = Math.round((Number(progress?.offset) || 0) * 100);
      const chapterTitle = progress?.chapterTitle ?? '';
      const position = t('chapterProgress', { index, total });
      return chapterTitle === '' ? `${position} · ${percent}%` : `${position} · ${chapterTitle} ${percent}%`;
    }

    function formatBytes(bytes) {
      if (typeof bytes !== 'number' || bytes <= 0) return '';
      const units = ['B', 'KB', 'MB', 'GB'];
      let value = bytes;
      let unit = 0;
      while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit += 1;
      }
      return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
    }

    // ── job status ──────────────────────────────────────────────────────────

    function jobStatusChip(job) {
      if (job === null || job === undefined) return null;
      if (job.status === 'running') {
        return h('span', { className: 'nr-chip nr-chip--warn' }, [h(Spinner, { key: 's' }), h('span', { key: 'l' }, t('jobRunning'))]);
      }
      if (job.status === 'done') return h('span', { className: 'nr-chip nr-chip--ok' }, t('jobDone'));
      if (job.status === 'cancelled') return h('span', { className: 'nr-chip' }, t('jobCancelled'));
      return h('span', { className: 'nr-chip nr-chip--bad' }, t('jobFailed'));
    }

    function JobPanel({ job, onCancel, onDismiss }) {
      const [logsOpen, setLogsOpen] = React.useState(false);
      if (job === null || job === undefined) return null;
      const { done, total, volume, title } = job.progress ?? {};
      const fraction = total > 0 ? done / total : 0;
      const message = job.status === 'done'
        ? describeResult(job)
        : job.status === 'failed'
          ? job.error ?? t('jobFailed')
          : job.status === 'cancelled'
            ? t('jobCancelled')
            : [volume, title].filter((part) => typeof part === 'string' && part !== '').join(' · ').trim();
      return h('div', { className: 'nr-panel' }, [
        h('div', { className: 'nr-panel-title', key: 'head' }, [
          h('span', { key: 'kind' }, job.kind === 'update' ? t('updating') : t('downloading')),
          h('span', { key: 'label', className: 'nr-subtle' }, job.label ?? ''),
          h('span', { className: 'nr-spacer', key: 'spacer' }),
          jobStatusChip(job),
        ]),
        total > 0 ? h(ProgressBar, { key: 'bar', fraction }) : null,
        total > 0
          ? h('div', { key: 'count', className: 'nr-card-meta' }, `${done}/${total}${message === '' ? '' : ` · ${message}`}`)
          : (message === '' ? null : h('div', { key: 'message', className: job.status === 'failed' ? 'nr-error' : 'nr-subtle' }, message)),
        h('div', { key: 'actions', className: 'nr-row' }, [
          job.status === 'running'
            ? h('button', { key: 'cancel', type: 'button', className: 'nr-btn', onClick: onCancel }, t('cancelJob'))
            : h('button', { key: 'dismiss', type: 'button', className: 'nr-btn nr-btn--ghost', onClick: onDismiss }, t('cancel')),
          job.logs && job.logs.length > 0
            ? h('button', {
              key: 'logs',
              type: 'button',
              className: 'nr-btn nr-btn--ghost',
              onClick: () => setLogsOpen((open) => !open),
            }, logsOpen ? t('hideLogs') : t('showLogs'))
            : null,
        ]),
        logsOpen && job.logs && job.logs.length > 0
          ? h('pre', { key: 'log', className: 'nr-job-logs' }, job.logs.map((line) => line.text).join('\n'))
          : null,
      ]);
    }

    function describeResult(job) {
      const result = job.result ?? {};
      if (job.kind === 'download' && typeof result.savedTo === 'string') {
        return t('savedTo', { path: result.savedTo });
      }
      if (job.kind === 'update') {
        return t('addedChapters', { n: Number(result.addedChapters) || 0, total: Number(result.totalChapters) || 0 });
      }
      if (job.kind === 'import') return t('imported', { n: Array.isArray(result) ? result.length : 0 });
      return '';
    }

    // ── library view ────────────────────────────────────────────────────────

    function LibraryView({ books, filter, onFilter, busy, onOpen, onUpdate, onReset, onDelete, onImport, importing }) {
      const [importPath, setImportPath] = React.useState('');
      const needle = filter.trim().toLowerCase();
      const rows = needle === ''
        ? books
        : books.filter((book) => `${book.title ?? ''} ${book.author ?? ''}`.toLowerCase().includes(needle));
      return h('div', null, [
        h('div', { key: 'filter', className: 'nr-search' }, [
          h('input', {
            key: 'input',
            className: 'nr-input',
            placeholder: t('filterPlaceholder'),
            value: filter,
            onChange: (event) => onFilter(event.target.value),
          }),
          h('span', { key: 'count', className: 'nr-chip' }, t('booksCount', { n: books.length })),
        ]),
        h('div', { key: 'import', className: 'nr-panel' }, [
          h('div', { className: 'nr-panel-title', key: 'title' }, t('importPath')),
          h('div', { className: 'nr-row', key: 'row' }, [
            h('input', {
              key: 'input',
              className: 'nr-input',
              placeholder: t('importPlaceholder'),
              value: importPath,
              onChange: (event) => setImportPath(event.target.value),
              onKeyDown: (event) => {
                if (event.key === 'Enter' && importPath.trim() !== '') onImport(importPath.trim());
              },
            }),
            h('button', {
              key: 'go',
              type: 'button',
              className: 'nr-btn',
              disabled: importing || importPath.trim() === '',
              onClick: () => onImport(importPath.trim()),
            }, importing ? t('loading') : t('importAction')),
          ]),
          h('div', { key: 'hint', className: 'nr-subtle' }, t('importHint')),
        ]),
        rows.length === 0
          ? h('div', { key: 'empty', className: 'nr-empty' }, busy ? t('loading') : t('emptyLibrary'))
          : h('ul', { key: 'list', className: 'nr-list' }, rows.map((book) => h('li', { key: book.id }, h('div', {
            className: 'nr-card',
            role: 'button',
            tabIndex: 0,
            onClick: () => onOpen(book),
            onKeyDown: (event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                onOpen(book);
              }
            },
          }, [
            h(Cover, { key: 'cover', book: withLocalCover(book) }),
            h('div', { key: 'main', className: 'nr-card-main' }, [
              h('div', { key: 'title', className: 'nr-card-title' }, book.title ?? ''),
              h('div', { key: 'meta', className: 'nr-card-meta' }, [
                h('span', { key: 'author' }, book.author ?? t('unknownAuthor')),
                h('span', { key: 'count' }, t('chaptersCount', { n: book.chapters ?? 0 })),
                book.present === false ? h('span', { key: 'missing', className: 'nr-chip nr-chip--bad' }, t('bookMissing')) : null,
                typeof book.sizeBytes === 'number' ? h('span', { key: 'size' }, formatBytes(book.sizeBytes)) : null,
              ]),
              h('div', { key: 'progress', className: 'nr-card-meta' }, progressText(book)),
              h(ProgressBar, { key: 'bar', fraction: bookFraction(book) }),
            ]),
            h('div', {
              key: 'actions',
              className: 'nr-card-actions',
              onClick: (event) => event.stopPropagation(),
            }, [
              h('button', {
                key: 'read',
                type: 'button',
                className: 'nr-btn nr-btn--primary',
                onClick: () => onOpen(book),
              }, t('read')),
              book.sourceId
                ? h('button', { key: 'update', type: 'button', className: 'nr-btn', onClick: () => onUpdate(book) }, t('update'))
                : null,
              h('button', { key: 'reset', type: 'button', className: 'nr-btn nr-btn--ghost', onClick: () => onReset(book) }, t('reset')),
              h('button', { key: 'delete', type: 'button', className: 'nr-btn nr-btn--ghost nr-btn--danger', onClick: () => onDelete(book) }, t('delete')),
            ]),
          ]))),
        ),
      ]);
    }

    // ── search view ─────────────────────────────────────────────────────────

    function ResultCard({ book, onDetail, onDownload, downloaded }) {
      const meta = [book.author, book.status, book.rating ? `★ ${book.rating}` : '', book.publisher]
        .filter((part) => typeof part === 'string' && part !== '');
      return h('li', null, h('div', { className: 'nr-card', style: { alignItems: 'flex-start' } }, [
        h(Cover, { key: 'cover', book }),
        h('div', { key: 'main', className: 'nr-card-main' }, [
          h('div', { key: 'title', className: 'nr-card-title' }, book.title ?? ''),
          h('div', { key: 'meta', className: 'nr-card-meta' }, [
            h('span', { key: 'meta' }, meta.join(' · ')),
            downloaded ? h('span', { key: 'in', className: 'nr-chip nr-chip--ok' }, t('inLibrary')) : null,
            book.novelId !== '' ? h('span', { key: 'id', className: 'nr-chip' }, `#${book.novelId}`) : null,
          ]),
          (book.description ?? '') !== ''
            ? h('div', { key: 'desc', className: 'nr-subtle', style: { maxWidth: 'none', whiteSpace: 'normal' } }, shorten(book.description, 160))
            : null,
        ]),
        h('div', { key: 'actions', className: 'nr-card-actions' }, [
          h('button', { key: 'detail', type: 'button', className: 'nr-btn', onClick: () => onDetail(book) }, t('detail')),
          h('button', { key: 'download', type: 'button', className: 'nr-btn nr-btn--primary', onClick: () => onDownload(book) }, t('download')),
        ]),
      ]));
    }

    /**
     * Resolve one chapter block's image source: archive entries are served by
     * this plugin, while data/remote sources are used as written.
     * @param {string} bookId
     * @param {string} src
     */
    function imageSource(bookId, src) {
      const value = String(src ?? '');
      if (/^(?:data:|blob:|https?:)/i.test(value)) return value;
      return api.imageUrl(bookId, value);
    }

    /** Prefer the book's own cover over a remote search thumbnail. */
    function withLocalCover(book) {
      const isEpub = String(book?.fileType ?? '').toLowerCase() === 'epub';
      if (!isEpub || book?.present === false) return book;
      return { ...book, coverUrl: api.coverUrl(book.id) };
    }

    function shorten(text, limit) {
      const characters = Array.from(String(text));
      return characters.length <= limit ? String(text) : `${characters.slice(0, limit).join('')}…`;
    }

    // ── detail view ─────────────────────────────────────────────────────────

    function DetailView({ detail, loading, error, onBack, onDownload, onUpdate, onRead, busy }) {
      if (loading) return h('div', { className: 'nr-empty' }, t('loading'));
      if (error !== null) return h('div', null, [
        h('div', { key: 'back', className: 'nr-search' }, h('button', {
          key: 'b', type: 'button', className: 'nr-btn', onClick: onBack,
        }, `← ${t('back')}`)),
        h(ErrorLine, { key: 'error', error }),
      ]);
      if (detail === null) return null;
      const book = detail.book ?? {};
      const downloaded = detail.downloaded;
      const meta = [book.author, book.status, book.rating ? `★ ${book.rating}` : '', book.publisher]
        .filter((part) => typeof part === 'string' && part !== '');
      return h('div', null, [
        h('div', { key: 'bar', className: 'nr-search' }, [
          h('button', { key: 'back', type: 'button', className: 'nr-btn nr-btn--ghost', onClick: onBack }, `← ${t('back')}`),
          h('span', { className: 'nr-spacer', key: 'spacer' }),
          book.novelId !== '' && book.novelId !== undefined ? h('span', { key: 'id', className: 'nr-chip' }, `#${book.novelId}`) : null,
          downloaded
            ? h('button', { key: 'read', type: 'button', className: 'nr-btn', onClick: () => onRead(downloaded) }, t('read'))
            : null,
          downloaded
            ? h('button', { key: 'update', type: 'button', className: 'nr-btn', disabled: busy, onClick: onUpdate }, busy ? t('updating') : t('update'))
            : h('button', { key: 'download', type: 'button', className: 'nr-btn nr-btn--primary', disabled: busy, onClick: onDownload }, busy ? t('downloading') : t('download')),
        ]),
        h('div', { key: 'head', className: 'nr-detail-head' }, [
          h(Cover, { key: 'cover', book }),
          h('div', { key: 'meta', className: 'nr-detail-meta' }, [
            h('div', { key: 'title', style: { fontSize: '17px', fontWeight: 500 } }, book.title ?? ''),
            h('div', { key: 'meta', className: 'nr-card-meta' }, meta.join(' · ')),
            h('div', { key: 'count', className: 'nr-card-meta' }, [
              h('span', { key: 'chapters' }, t('chaptersCount', { n: detail.totalChapters ?? 0 })),
              h('span', { key: 'volumes' }, t('volumesCount', { n: (detail.volumes ?? []).length })),
              downloaded
                ? h('span', { key: 'in', className: 'nr-chip nr-chip--ok' }, t('inLibrary'))
                : h('span', { key: 'out', className: 'nr-chip' }, t('notInLibrary')),
            ]),
            (book.tags ?? '') !== '' ? h('div', { key: 'tags', className: 'nr-card-meta' }, `${t('tags')}: ${book.tags}`) : null,
          ]),
        ]),
        (book.description ?? '') !== ''
          ? h('section', { key: 'desc' }, [
            h('h3', { key: 'title', className: 'nr-section', style: { marginTop: '16px' } }, t('introduction')),
            h('div', { key: 'body', className: 'nr-desc' }, book.description),
          ])
          : null,
        (detail.volumes ?? []).length > 0
          ? h('section', { key: 'catalog' }, [
            h('h3', { key: 'title', className: 'nr-section', style: { marginTop: '16px' } }, `${t('catalog')} · ${t('volumesCount', { n: detail.volumes.length })}`),
            h('div', { key: 'list', className: 'nr-volumes' }, detail.volumes.map((volume, index) => h('details', {
              key: `${index}-${volume.title}`,
              className: 'nr-volume',
              open: index === 0,
            }, [
              h('summary', { key: 'summary' }, [
                h('span', { key: 'title' }, volume.title === '' ? t('chapters') : volume.title),
                h('span', { key: 'count', className: 'nr-chip' }, t('chaptersCount', { n: volume.chapters.length })),
              ]),
              h('ol', { key: 'chapters' }, volume.chapters.map((chapter, chapterIndex) => h('li', {
                key: `${chapterIndex}`,
              }, chapter.title))),
            ]))),
          ])
          : null,
      ]);
    }

    // ── reader ──────────────────────────────────────────────────────────────

    /** Gap between page columns; also the distance a flip travels minus one page. */
    const READER_GAP = 88;
    /** Widest line measure a page may use, so a wide panel does not stretch lines. */
    const READER_MAX_MEASURE = 860;
    const READER_PAD_X = 36;
    const READER_PAD_Y = 10;

    /**
     * How many pages a laid-out column strip holds. Each page advances by
     * `width + gap`, so a strip of N pages is `N * (width + gap) - gap` wide.
     * Rounded rather than floored because the browser returns fractional
     * layout widths; malformed or zero widths still mean one page.
     * @param {number} scrollWidth - measured strip width.
     * @param {number} width - one page's column width.
     * @param {number} gap - gap between columns.
     * @returns {number} page count, at least 1.
     */
    function pagesInStrip(scrollWidth, width, gap) {
      if (!Number.isFinite(scrollWidth) || scrollWidth <= 0 || !Number.isFinite(width) || width <= 0) return 1;
      return Math.max(1, Math.round((scrollWidth + gap) / (width + gap)));
    }

    /**
     * A page-turning reader: the chapter is laid out as one column per page and
     * a flip translates the column strip, so the reading area never scrolls and
     * a page always ends on a whole text line (the browser breaks columns
     * between line boxes, never through one).
     */
    function Reader({ book, onExit, initialChapter, initialOffset }) {
      const [chapters, setChapters] = React.useState(null);
      const [index, setIndex] = React.useState(Math.max(0, Number(book?.progress?.chapterIndex) || 0));
      const [chapter, setChapter] = React.useState(null);
      const [panelOpen, setPanelOpen] = React.useState(false);
      const [error, setError] = React.useState(null);
      const [layout, setLayout] = React.useState({ width: 0, height: 0, pages: 1 });
      const [currentPage, setCurrentPage] = React.useState(0);

      const viewportRef = React.useRef(null);
      const clipRef = React.useRef(null);
      const flowRef = React.useRef(null);
      const indexRef = React.useRef(index);
      const chapterRef = React.useRef(null);
      const pageRef = React.useRef(0);
      const pagesRef = React.useRef(1);
      /** Fraction of the chapter at the current page: the value the library stores. */
      const fractionRef = React.useRef(Number(initialOffset ?? book?.progress?.offset ?? 0) || 0);
      /** Where the next rendered chapter starts; undefined keeps the fraction. */
      const entryRef = React.useRef(undefined);
      const savedRef = React.useRef({ chapterIndex: -1, offset: -1 });
      const wheelAt = React.useRef(0);
      const relayoutTimer = React.useRef(null);

      indexRef.current = index;
      chapterRef.current = chapter;
      pageRef.current = currentPage;
      pagesRef.current = layout.pages;

      /**
       * Measure the viewport, lay the chapter out as columns, and settle on the
       * requested page. `mode` is 'first', 'last', or undefined to keep the
       * fraction we were on (a resize or the chapter panel opening).
       */
      const relayout = React.useCallback((mode) => {
        const viewport = viewportRef.current;
        const clip = clipRef.current;
        const flow = flowRef.current;
        if (viewport === null || clip === null || flow === null) return;
        const availableWidth = Math.max(240, viewport.clientWidth);
        const availableHeight = Math.max(160, viewport.clientHeight);
        const width = Math.max(180, Math.min(availableWidth - READER_PAD_X * 2, READER_MAX_MEASURE));
        const height = Math.max(120, availableHeight - READER_PAD_Y * 2);
        // The frame is exactly one page wide, so only this page's column shows.
        clip.style.width = `${width}px`;
        clip.style.height = `${height}px`;
        clip.style.left = `${Math.max(READER_PAD_X, Math.round((availableWidth - width) / 2))}px`;
        clip.style.top = `${READER_PAD_Y}px`;
        flow.style.width = `${width}px`;
        flow.style.height = `${height}px`;
        flow.style.columnWidth = `${width}px`;
        flow.style.columnGap = `${READER_GAP}px`;
        flow.style.left = '0px';
        flow.style.top = '0px';
        // Illustrations size themselves against one page's height.
        flow.style.setProperty('--nr-page-height', `${height}px`);
        // scrollWidth spans every column the browser laid out, so it counts pages.
        const pages = pagesInStrip(Math.max(width, flow.scrollWidth), width, READER_GAP);
        const fraction = Math.max(0, Math.min(1, fractionRef.current));
        let target;
        if (mode === 'first') target = 0;
        else if (mode === 'last') target = pages - 1;
        else target = Math.round(fraction * (pages - 1));
        target = Math.max(0, Math.min(pages - 1, target));
        // A re-page (resize, chapter panel, a late illustration) keeps the
        // fraction the reader was aiming at; quantizing it here would let
        // successive re-pages walk the position away. Only an explicit jump to
        // the first/last page redefines it.
        if (mode === 'first' || mode === 'last') {
          fractionRef.current = pages <= 1 ? 0 : target / (pages - 1);
        }
        setLayout({ width, height, pages });
        setCurrentPage(target);
      }, []);

      /** Persist the current chapter and page fraction; a repeated save is skipped. */
      const save = React.useCallback((immediate) => {
        const chapterIndex = indexRef.current;
        const offset = Math.max(0, Math.min(1, fractionRef.current));
        const title = chapterRef.current?.title ?? '';
        if (!immediate && savedRef.current.chapterIndex === chapterIndex && Math.abs(savedRef.current.offset - offset) < 0.001) {
          return;
        }
        savedRef.current = { chapterIndex, offset };
        api.saveProgress(book.id, { chapterIndex, offset, chapterTitle: title }).catch(() => { /* retried on the next tick */ });
      }, [book.id]);

      // Chapter catalog; also settles the starting chapter from the shared file.
      React.useEffect(() => {
        let live = true;
        api.chapters(book.id).then((payload) => {
          if (!live) return;
          setChapters(payload.chapters ?? []);
          const total = payload.chapters?.length ?? 0;
          // The list snapshot can be older than the shared file (the terminal
          // reader may have advanced since); prefer the position read now.
          const stored = Number(payload.progress?.chapterIndex ?? book?.progress?.chapterIndex) || 0;
          const storedOffset = Number(payload.progress?.offset ?? book?.progress?.offset) || 0;
          if (entryRef.current === undefined && stored > 0) {
            fractionRef.current = Math.max(0, Math.min(1, storedOffset));
            setIndex(Math.min(stored, Math.max(0, total - 1)));
          }
        }).catch((failure) => {
          if (live) setError(failure instanceof Error ? failure.message : String(failure));
        });
        return () => { live = false; };
      }, [book.id]);

      // One chapter's text.
      React.useEffect(() => {
        let live = true;
        setChapter(null);
        setError(null);
        api.chapter(book.id, index).then((payload) => {
          if (live) setChapter(payload);
        }).catch((failure) => {
          if (live) setError(failure instanceof Error ? failure.message : String(failure));
        });
        return () => { live = false; };
      }, [book.id, index]);

      // Lay out once the chapter's DOM exists, at the page the caller asked for.
      React.useLayoutEffect(() => {
        if (chapter === null) return;
        const mode = entryRef.current;
        entryRef.current = undefined;
        relayout(mode);
      }, [chapter, panelOpen, relayout]);

      // Re-page on every viewport change (window resize, chapter panel, dock).
      React.useEffect(() => {
        const viewport = viewportRef.current;
        if (viewport === null || typeof ResizeObserver === 'undefined') return;
        let frame = 0;
        const observer = new ResizeObserver(() => {
          if (frame !== 0) cancelAnimationFrame(frame);
          frame = requestAnimationFrame(() => { frame = 0; relayout(undefined); });
        });
        observer.observe(viewport);
        return () => {
          if (frame !== 0) cancelAnimationFrame(frame);
          observer.disconnect();
        };
      }, [relayout]);

      // A late webfont changes line height, so re-page once fonts are ready.
      React.useEffect(() => {
        const fonts = document.fonts;
        if (fonts === undefined || typeof fonts.ready?.then !== 'function') return;
        let live = true;
        fonts.ready.then(() => { if (live) relayout(undefined); }).catch(() => {});
        return () => { live = false; };
      }, [relayout]);

      /** Re-page once late-loading illustrations have taken their space. */
      const scheduleRelayout = React.useCallback(() => {
        if (relayoutTimer.current !== null) clearTimeout(relayoutTimer.current);
        relayoutTimer.current = setTimeout(() => {
          relayoutTimer.current = null;
          relayout(undefined);
        }, 140);
      }, [relayout]);

      React.useEffect(() => () => {
        if (relayoutTimer.current !== null) clearTimeout(relayoutTimer.current);
      }, []);

      const turnChapter = React.useCallback((delta) => {
        const total = chapters?.length ?? 0;
        setIndex((current) => {
          const next = Math.max(0, Math.min(total - 1, current + delta));
          if (next === current) return current;
          save(true);
          entryRef.current = delta > 0 ? 'first' : 'last';
          return next;
        });
      }, [chapters, save]);

      /** One page forward/back; at either end of the chapter, change chapter. */
      const flip = React.useCallback((direction) => {
        const pages = pagesRef.current;
        const next = pageRef.current + direction;
        if (next < 0) {
          turnChapter(-1);
          return;
        }
        if (next > pages - 1) {
          turnChapter(1);
          return;
        }
        pageRef.current = next;
        fractionRef.current = pages <= 1 ? 0 : next / (pages - 1);
        setCurrentPage(next);
        save(false);
      }, [turnChapter, save]);

      // Saving rides the page/ chapter changes plus a slow tick for idle reading.
      React.useEffect(() => {
        const timer = setInterval(() => save(false), 10000);
        const onHide = () => save(true);
        window.addEventListener('pagehide', onHide);
        document.addEventListener('visibilitychange', onHide);
        return () => {
          clearInterval(timer);
          window.removeEventListener('pagehide', onHide);
          document.removeEventListener('visibilitychange', onHide);
          save(true);
        };
      }, [save]);

      const jumpTo = React.useCallback((target) => {
        entryRef.current = 'first';
        setIndex(target);
        setPanelOpen(false);
      }, []);

      // Keys: page turns, chapter jumps, catalog, exit. No scrolling anywhere.
      React.useEffect(() => {
        const handler = (event) => {
          if (event.metaKey || event.ctrlKey || event.altKey) return;
          switch (event.key) {
            case 'ArrowRight':
            case 'PageDown':
            case 'l':
            case 'j':
              event.preventDefault();
              flip(1);
              break;
            case 'ArrowLeft':
            case 'PageUp':
            case 'h':
            case 'k':
              event.preventDefault();
              flip(-1);
              break;
            case 'n':
              event.preventDefault();
              turnChapter(1);
              break;
            case 'p':
              event.preventDefault();
              turnChapter(-1);
              break;
            case 's':
              event.preventDefault();
              setPanelOpen((open) => !open);
              break;
            case 'q':
            case 'Escape':
              event.preventDefault();
              save(true);
              onExit();
              break;
            default:
              break;
          }
        };
        window.addEventListener('keydown', handler);
        return () => window.removeEventListener('keydown', handler);
      }, [flip, turnChapter, onExit, save]);

      // The wheel flips pages instead of scrolling the text.
      React.useEffect(() => {
        const viewport = viewportRef.current;
        if (viewport === null) return;
        const onWheel = (event) => {
          event.preventDefault();
          if (Math.abs(event.deltaY) < 2) return;
          const now = Date.now();
          if (now - wheelAt.current < 170) return;
          wheelAt.current = now;
          flip(event.deltaY > 0 ? 1 : -1);
        };
        viewport.addEventListener('wheel', onWheel, { passive: false });
        return () => viewport.removeEventListener('wheel', onWheel);
      }, [flip]);

      const total = chapters?.length ?? 0;
      const pages = layout.pages;
      const percent = pages <= 1 ? 0 : Math.round((currentPage / (pages - 1)) * 100);
      const chapterTitle = chapter?.title ?? '…';
      const onClickStage = (event) => {
        const viewport = viewportRef.current;
        if (viewport === null) return;
        if (String(window.getSelection?.() ?? '') !== '') return;
        const rect = viewport.getBoundingClientRect();
        const x = event.clientX - rect.left;
        if (x < rect.width / 3) flip(-1);
        else if (x > (rect.width * 2) / 3) flip(1);
      };

      return h('div', { className: 'nr-reader' }, [
        h('header', { key: 'head', className: 'nr-reader-head' }, [
          h('button', {
            key: 'back',
            type: 'button',
            className: 'nr-btn nr-btn--ghost',
            onClick: () => {
              save(true);
              onExit();
            },
          }, `← ${t('backToLibrary')}`),
          h('div', { key: 'title', className: 'nr-reader-title' }, [
            h('div', { key: 'book', className: 'nr-reader-book' }, book.title ?? ''),
            h('div', { key: 'chapter', className: 'nr-reader-chapter' }, `${t('chapters')} ${index + 1}/${total || '?'} · ${chapterTitle}`),
          ]),
          h('div', { key: 'actions', className: 'nr-row' }, [
            h('button', { key: 'prev', type: 'button', className: 'nr-btn', onClick: () => turnChapter(-1), title: t('prevChapter') }, '«'),
            h('button', { key: 'prevpage', type: 'button', className: 'nr-btn', onClick: () => flip(-1), title: t('prevPage') }, '‹'),
            h('button', { key: 'nextpage', type: 'button', className: 'nr-btn', onClick: () => flip(1), title: t('nextPage') }, '›'),
            h('button', { key: 'next', type: 'button', className: 'nr-btn', onClick: () => turnChapter(1), title: t('nextChapter') }, '»'),
            h('button', {
              key: 'panel',
              type: 'button',
              className: panelOpen ? 'nr-btn nr-btn--primary' : 'nr-btn',
              onClick: () => setPanelOpen((open) => !open),
            }, t('chapterPanel')),
          ]),
        ]),
        h('div', { key: 'stage', className: 'nr-reader-stage' }, [
          h('div', {
            key: 'viewport',
            className: 'nr-reader-viewport',
            ref: viewportRef,
            onClick: onClickStage,
          }, error !== null
            ? h('div', { key: 'error', className: 'nr-error', style: { padding: '24px' } }, error)
            : chapter === null
              ? h('div', { key: 'loading', className: 'nr-empty' }, [h(Spinner, { key: 's' }), h('span', { key: 'text' }, t('loadingChapter'))])
              : h('div', { key: 'clip', className: 'nr-page-clip', ref: clipRef }, h('div', {
                key: 'flow',
                className: 'nr-reader-flow',
                ref: flowRef,
                style: { transform: `translate3d(-${Math.round(currentPage * (layout.width + READER_GAP))}px, 0, 0)` },
              }, h('article', { className: 'nr-article' }, [
                ...(chapter.blocks ?? chapter.paragraphs.map((text) => ({ type: 'text', text }))).map((block, blockIndex) => (
                  block.type === 'image'
                    ? h('figure', { key: blockIndex }, h('img', {
                      key: 'img',
                      src: imageSource(book.id, block.src),
                      alt: block.alt ?? '',
                      // Eager: a lazy image reports zero height until it enters
                      // the viewport, which would make the page count drift as
                      // the reader flips into it.
                      loading: 'eager',
                      decoding: 'async',
                      onLoad: scheduleRelayout,
                      onError: (event) => { event.currentTarget.style.display = 'none'; },
                    }))
                    : h('p', { key: blockIndex }, block.text)
                )),
                h('p', { key: 'end', className: 'nr-reader-end' }, `— ${t('readerEnd')} —`),
              ])))),
          chapter !== null && error === null
            ? h('div', { key: 'hint', className: 'nr-page-hint', 'data-few': pages <= 1 ? 'true' : 'false' }, `${currentPage + 1} / ${pages}`)
            : null,
          panelOpen
            ? h('aside', { key: 'chapters', className: 'nr-chapters' }, [
              h('div', { key: 'head', className: 'nr-chapters-head' }, [
                h('span', { key: 'title' }, `${t('chapters')} · ${total}`),
                h('button', { key: 'close', type: 'button', className: 'nr-btn nr-btn--ghost', onClick: () => setPanelOpen(false) }, '×'),
              ]),
              ...(chapters ?? []).map((entry) => h('button', {
                key: entry.index,
                type: 'button',
                className: 'nr-chapter',
                'data-current': entry.index === index ? 'true' : 'false',
                ref: entry.index === index
                  ? (element) => {
                    if (element !== null && panelOpen && typeof element.scrollIntoView === 'function') {
                      element.scrollIntoView({ block: 'nearest' });
                    }
                  }
                  : undefined,
                onClick: () => jumpTo(entry.index),
              }, [
                h('span', { key: 'index', className: 'nr-chapter-index' }, `${entry.index + 1}`),
                h('span', { key: 'title' }, entry.title),
              ])),
            ])
            : null,
        ]),
        h('footer', { key: 'foot', className: 'nr-reader-foot' }, [
          h('span', { key: 'position' }, t('position', { index: index + 1, total: total || '?', percent })),
          h('span', { key: 'page' }, t('pageOf', { page: currentPage + 1, pages })),
          h('span', { key: 'spacer', className: 'nr-spacer' }),
          h('span', { key: 'hint' }, t('pageHint')),
        ]),
      ]);
    }

    // ── page ────────────────────────────────────────────────────────────────

    function NovelReaderPage() {
      const [tab, setTab] = React.useState('library');
      const [status, setStatus] = React.useState(null);
      const [books, setBooks] = React.useState([]);
      const [booksBusy, setBooksBusy] = React.useState(true);
      const [booksError, setBooksError] = React.useState(null);
      const [filter, setFilter] = React.useState('');
      const [query, setQuery] = React.useState('');
      const [search, setSearch] = React.useState({ status: 'idle', books: [], suggestion: '', error: null, for: '' });
      const [detail, setDetail] = React.useState(null);
      const [detailQuery, setDetailQuery] = React.useState(null);
      const [detailState, setDetailState] = React.useState({ loading: false, error: null });
      const [job, setJob] = React.useState(null);
      const [importing, setImporting] = React.useState(false);
      const [toast, setToast] = React.useState(null);
      const [reader, setReader] = React.useState(null);
      const searchToken = React.useRef(0);

      const notify = React.useCallback((message) => {
        setToast(message);
        window.setTimeout(() => setToast((current) => (current === message ? null : current)), 6000);
      }, []);

      const refreshBooks = React.useCallback(async () => {
        setBooksBusy(true);
        try {
          const payload = await api.books();
          setBooks(payload.books ?? []);
          setBooksError(null);
        } catch (error) {
          setBooksError(error instanceof Error ? error.message : String(error));
        } finally {
          setBooksBusy(false);
        }
      }, []);

      React.useEffect(() => {
        refreshBooks();
        api.status().then(setStatus).catch(() => setStatus(null));
      }, [refreshBooks]);

      // Debounced search: 500 ms after the last keystroke, like the TUI.
      React.useEffect(() => {
        const text = query.trim();
        if (text === '') {
          setSearch({ status: 'idle', books: [], suggestion: '', error: null, for: '' });
          return;
        }
        const timer = setTimeout(() => runSearch(text), 500);
        return () => clearTimeout(timer);
      }, [query]);

      const runSearch = React.useCallback(async (text) => {
        const token = ++searchToken.current;
        setSearch({ status: 'loading', books: [], suggestion: '', error: null, for: text });
        try {
          const payload = await api.search(text);
          if (searchToken.current !== token) return;
          setSearch({
            status: 'done',
            books: payload.books ?? [],
            suggestion: payload.suggestion ?? '',
            error: null,
            for: text,
          });
        } catch (error) {
          if (searchToken.current !== token) return;
          setSearch({ status: 'error', books: [], suggestion: '', error: error instanceof Error ? error.message : String(error), for: text });
        }
      }, []);

      // Poll the running job until it settles.
      React.useEffect(() => {
        if (job === null || job.id === undefined) return;
        if (job.status !== 'running') return;
        let live = true;
        const timer = setTimeout(async () => {
          try {
            const next = await api.job(job.id);
            if (!live) return;
            setJob(next);
            if (next.status !== 'running') {
              if (next.status === 'done') {
                refreshBooks();
                if (next.kind === 'import' && Array.isArray(next.result)) notify(t('imported', { n: next.result.length }));
                // A finished download changes the detail page's state too
                // ("未下载" → "已在书库"), so re-read it while it is open.
                setDetailQuery((query) => {
                  if (query !== null) {
                    api.detail(query)
                      .then((payload) => setDetail(payload))
                      .catch(() => {});
                  }
                  return query;
                });
              }
            }
          } catch {
            if (!live) return;
            setJob((current) => (current === null ? null : { ...current, status: 'failed', error: t('jobFailed') }));
          }
        }, 700);
        return () => {
          live = false;
          clearTimeout(timer);
        };
      }, [job, refreshBooks, notify]);

      const openBook = React.useCallback((book) => {
        setReader({ book, chapter: null, offset: null });
        setDetail(null);
      }, []);

      const startJob = React.useCallback(async (kind, text) => {
        try {
          const created = kind === 'update' ? await api.update(text) : await api.download(text);
          setJob(created);
        } catch (error) {
          notify(error instanceof Error ? error.message : String(error));
        }
      }, [notify]);

      const openDetail = React.useCallback(async (text) => {
        setTab('search');
        setDetailQuery(text);
        setDetailState({ loading: true, error: null });
        setDetail(null);
        try {
          const payload = await api.detail(text);
          setDetail(payload);
          setDetailState({ loading: false, error: null });
        } catch (error) {
          setDetailState({ loading: false, error: error instanceof Error ? error.message : String(error) });
        }
      }, []);

      const onReset = React.useCallback(async (book) => {
        if (!window.confirm(t('confirmReset'))) return;
        try {
          await api.resetProgress(book.id);
          await refreshBooks();
          notify(t('reset'));
        } catch (error) {
          notify(error instanceof Error ? error.message : String(error));
        }
      }, [refreshBooks, notify]);

      const onDelete = React.useCallback(async (book) => {
        if (!window.confirm(t('confirmDelete'))) return;
        try {
          await api.deleteBook(book.id);
          await refreshBooks();
        } catch (error) {
          notify(error instanceof Error ? error.message : String(error));
        }
      }, [refreshBooks, notify]);

      const onImport = React.useCallback(async (path) => {
        setImporting(true);
        try {
          const payload = await api.importPath(path);
          await refreshBooks();
          notify(t('imported', { n: payload.imported?.length ?? 0 }));
        } catch (error) {
          notify(error instanceof Error ? error.message : String(error));
        } finally {
          setImporting(false);
        }
      }, [refreshBooks, notify]);

      const cancelJob = React.useCallback(async () => {
        if (job === null) return;
        try {
          const payload = await api.cancelJob(job.id);
          if (payload.job !== undefined) setJob(payload.job);
        } catch (error) {
          notify(error instanceof Error ? error.message : String(error));
        }
      }, [job, notify]);

      if (reader !== null) {
        return h(Reader, {
          book: reader.book,
          initialChapter: reader.chapter,
          initialOffset: reader.offset,
          onExit: () => {
            setReader(null);
            refreshBooks();
          },
          onNotify: notify,
        });
      }

      const downloadedSources = new Set(books.map((book) => book.sourceId).filter((id) => typeof id === 'string' && id !== ''));
      const detailInLibrary = detail?.downloaded ?? null;
      /** A key the CLI resolves: a novel id/URL for download, a library id or title for update. */
      const detailTarget = detail?.book?.novelId !== undefined && detail?.book?.novelId !== ''
        ? detail.book.novelId
        : (detail?.book?.url ?? query.trim());

      return h('div', { className: 'nr-page' }, [
        h('header', { key: 'head', className: 'nr-head' }, [
          h('div', { key: 'title', className: 'nr-title-row' }, [
            h('h1', { key: 'h1', className: 'nr-title' }, t('title')),
            status !== null && status.dataDir !== undefined
              ? h('span', { key: 'dir', className: 'nr-subtle', title: status.dataDir }, status.dataDir)
              : null,
            h('span', { key: 'spacer', className: 'nr-spacer' }),
            status !== null && status.cli !== undefined && status.cli.available === false
              ? h('span', { key: 'cli', className: 'nr-chip nr-chip--warn', title: status.cli.error ?? '' }, t('cliMissing'))
              : null,
            status !== null && status.cli?.version
              ? h('span', { key: 'version', className: 'nr-chip' }, String(status.cli.version))
              : null,
            h('button', {
              key: 'refresh',
              type: 'button',
              className: 'nr-btn nr-btn--ghost',
              onClick: () => {
                refreshBooks();
                api.status().then(setStatus).catch(() => {});
              },
            }, t('refresh')),
          ]),
          h('div', { key: 'tabs', className: 'nr-tabs' }, [
            h('button', {
              key: 'library',
              type: 'button',
              className: 'nr-tab',
              'data-active': tab === 'library' ? 'true' : 'false',
              onClick: () => setTab('library'),
            }, `${t('tabLibrary')}${books.length > 0 ? ` (${books.length})` : ''}`),
            h('button', {
              key: 'search',
              type: 'button',
              className: 'nr-tab',
              'data-active': tab === 'search' ? 'true' : 'false',
              onClick: () => setTab('search'),
            }, t('tabSearch')),
          ]),
        ]),
        h('div', { key: 'body', className: 'nr-body' }, tab === 'library'
          ? h(LibraryView, {
            books,
            filter,
            onFilter: setFilter,
            busy: booksBusy,
            importing,
            onOpen: openBook,
            onUpdate: (book) => startJob('update', book.id),
            onReset,
            onDelete,
            onImport,
          })
          : h('div', null, [
            h('div', { key: 'search', className: 'nr-search' }, [
              h('input', {
                key: 'input',
                className: 'nr-input',
                autoFocus: true,
                placeholder: t('emptySearch'),
                value: query,
                onChange: (event) => setQuery(event.target.value),
                onKeyDown: (event) => {
                  if (event.key === 'Enter' && query.trim() !== '') {
                    event.preventDefault();
                    runSearch(query.trim());
                  }
                },
              }),
              h('button', {
                key: 'go',
                type: 'button',
                className: 'nr-btn',
                onClick: () => {
                  if (query.trim() !== '') runSearch(query.trim());
                },
              }, t('search')),
              h('button', {
                key: 'detail',
                type: 'button',
                className: 'nr-btn',
                disabled: query.trim() === '',
                onClick: () => openDetail(query.trim()),
              }, t('detail')),
            ]),
            h('div', { key: 'hint', className: 'nr-subtle', style: { marginBottom: '10px' } }, t('searchHint')),
            h(JobPanel, {
              key: 'job',
              job,
              onCancel: cancelJob,
              onDismiss: () => setJob(null),
            }),
            booksError !== null ? h(ErrorLine, { key: 'booksError', error: booksError, onDismiss: () => setBooksError(null) }) : null,
            detailState.loading || detailState.error !== null || detail !== null
              ? h(DetailView, {
                key: 'detail',
                detail,
                loading: detailState.loading,
                error: detailState.error,
                onBack: () => {
                  setDetail(null);
                  setDetailQuery(null);
                  setDetailState({ loading: false, error: null });
                },
                busy: job !== null && job.status === 'running',
                onDownload: () => startJob('download', detailTarget),
                onUpdate: () => startJob('update', String(detailInLibrary?.id ?? detail?.book?.title ?? detailTarget)),
                onRead: (downloaded) => {
                  const book = books.find((entry) => entry.id === downloaded.id);
                  if (book !== undefined) openBook(book);
                  else refreshBooks();
                },
              })
              : h('div', null, [
                h('h3', { key: 'results', className: 'nr-section' }, search.status === 'done'
                  ? `${t('tabSearch')} · ${search.books.length}`
                  : ''),
                search.status === 'error'
                  ? h(ErrorLine, { key: 'error', error: `${t('searchFailed')}: ${search.error}` })
                  : null,
                search.status === 'loading'
                  ? h('div', { key: 'loading', className: 'nr-empty' }, [h(Spinner, { key: 's' }), h('span', { key: 'text' }, t('searching'))])
                  : null,
                search.status === 'idle'
                  ? h('div', { key: 'idle', className: 'nr-empty' }, t('emptySearch'))
                  : null,
                search.status === 'done' && search.books.length === 0
                  ? h('div', { key: 'none', className: 'nr-empty' }, t('noResults'))
                  : null,
                search.suggestion !== ''
                  ? h('div', { key: 'suggestion', className: 'nr-subtle' }, `did you mean: ${search.suggestion}`)
                  : null,
                h('ul', { key: 'list', className: 'nr-list' }, search.books.map((book, index) => h(ResultCard, {
                  key: `${book.novelId || index}`,
                  book,
                  downloaded: book.novelId !== '' && downloadedSources.has(book.novelId),
                  onDetail: () => openDetail(book.novelId !== '' ? book.novelId : book.title),
                  onDownload: () => startJob('download', book.novelId !== '' ? book.novelId : book.title),
                }))),
              ]),
          ])),
        toast === null
          ? null
          : h('div', { key: 'toast', className: 'nr-toast' }, [
            h('span', { key: 'text' }, toast),
            h('button', {
              key: 'close',
              type: 'button',
              className: 'nr-btn nr-btn--ghost',
              onClick: () => setToast(null),
            }, '×'),
          ]),
      ]);
    }

    // ── registration ────────────────────────────────────────────────────────

    /** Services this browser plugin needs. */
    const inject = ['slots'];

    /**
     * Client plugin body: styles, the keyed main-panel page, and its
     * left-sidebar launcher.
     * @param {any} ctx - client root context.
     */
    function apply(ctx) {
      ctx.effect(() => installStyles(), 'novel-reader: styles');
      ctx.slots.inject('main', () => ctx.slots.register({
        name: 'main',
        key: PANEL_ID,
      }, NovelReaderPage));
      ctx.slots.inject('sidebar.panellist', () => ctx.slots.register({
        name: 'sidebar.panellist',
        id: PANEL_ID,
        order: 40,
        label: () => t('panel'),
      }, Icon));
    }

    // The page components are exported for local render tests; the plugin
    // contract is `apply` + `inject`, which are what Cordis consumes.
    return { apply, inject, NovelReaderPage, Reader, LibraryView, DetailView, Icon, pagesInStrip };
  },
});
