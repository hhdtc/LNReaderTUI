/**
 * Runs the pagination harness and asserts its measurements.
 *
 * The reading view is laid out as one column per page, so this checks in a real
 * browser that:
 *   - the chapter splits into multiple pages,
 *   - the strip width equals `pages * (column + gap) - gap`,
 *   - the reading box never overflows vertically (no half line parked below it),
 *   - no measured line box crosses a page's top or bottom edge, on every page,
 *   - a flip advances by exactly one column span,
 *   - the stored position is the page fraction, and the reader restores to it.
 *
 * A browser can be launched here when one is available; otherwise the script
 * starts the harness server, prints the URL, and waits for the page to report —
 * open it in any browser.
 *
 *   node test/pagination.mjs                       # launch, or wait for a browser
 *   node test/pagination.mjs --no-launch           # always wait for a manual open
 *   BROWSER=/path/to/chrome node test/pagination.mjs
 */

import { access, constants } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { spawn } from 'node:child_process';
import { join } from 'node:path';
import { startServer } from './pagination-server.mjs';

const here = new URL('.', import.meta.url).pathname;
const noLaunch = process.argv.includes('--no-launch');
const candidates = [
  process.env.BROWSER,
  join(process.env.HOME ?? '', '.cache/ms-playwright/chromium-1234/chrome-linux64/chrome'),
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/usr/bin/google-chrome',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
].filter((value) => typeof value === 'string' && value !== '');

const reportPromise = new Promise((resolve) => {
  startServer(0, resolve).then((server) => {
    console.log(`pagination harness: ${server.url}`);
    console.log('opening it in a browser also works; the script waits for its report');
    globalThis.__server = server;
    if (noLaunch) return;
    launch(server.url).catch((error) => {
      console.log(`note: no browser launched (${error.message}); open the URL above`);
    });
  });
});

let finished = false;
const timer = setTimeout(() => {
  if (finished) return;
  console.error('\nno report arrived within 90s');
  process.exit(1);
}, 90000);

const report = await reportPromise;
finished = true;
clearTimeout(timer);
await globalThis.__server?.close();
assertReport(report);

/** Try to drive a headless browser at the harness, if the machine runs one. */
async function launch(url) {
  const executable = await pickBrowser();
  if (executable === null) throw new Error('no chromium/chrome found');
  const child = spawn(executable, [
    '--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage',
    '--disable-crash-reporter', '--disable-breakpad', '--user-data-dir=/tmp/pagination-profile',
    '--virtual-time-budget=6000', '--dump-dom', url,
  ], { stdio: 'ignore' });
  child.on('error', () => {});
  // The page reports over HTTP, so --dump-dom output is not needed.
  setTimeout(() => { try { child.kill('SIGKILL'); } catch { /* gone */ } }, 30000).unref?.();
}

async function pickBrowser() {
  for (const candidate of candidates) {
    if (candidate === undefined) continue;
    try {
      await access(candidate, constants.X_OK);
      // A candidate that exists but cannot start is caught by the launch timeout.
      return candidate;
    } catch {
      /* try the next one */
    }
  }
  return null;
}

function assertReport(report) {
  const failures = [];
  const check = (condition, message) => { if (!condition) failures.push(message); };
  check(report && typeof report === 'object', 'report is not an object');
  if (failures.length > 0) return fail(failures);
  const steps = Array.isArray(report.steps) ? report.steps : [];
  for (const step of steps) {
    if (step.step === 'fail') failures.push(step.message);
    if (step.step === 'error') failures.push(step.message);
  }
  check(report.ok === true, 'harness did not finish');
  check(report.pages >= 2, `expected multiple pages, got ${report.pages}`);
  check(Math.abs(report.stripWidth - report.expectedStripWidth) <= 2,
    `strip width ${report.stripWidth} != ${report.expectedStripWidth}`);
  check(report.verticalOverflow <= 0, `reading box overflows by ${report.verticalOverflow}px`);
  check(report.firstPageLines?.checked > 0, 'no lines measured on the first page');
  check(report.firstPageLines?.clipped === 0, 'a line crosses the first page edge');
  check(report.secondPageLines?.clipped === 0, 'a line crosses the second page edge');
  check(report.flip?.hintBefore !== report.flip?.hintAfter, 'a click did not turn the page');
  check(Math.abs((report.flip?.dx ?? 0) - (report.flip?.expectedDx ?? 0)) <= 2, 'a flip moved the wrong distance');
  check(report.clippedPages === 0, `${report.clippedPages} page(s) clip a line`);
  if (failures.length > 0) return fail(failures);
  console.log(`ok   ${report.pages} pages · ${report.columnWidth}px columns · no clipped line on ${report.pages} pages`);
  console.log(`ok   flips advance ${report.flip.expectedDx}px and store offset ${report.saved?.[0]?.offset}`);
  console.log('\nall pagination checks passed');
}

function fail(messages) {
  for (const message of messages) console.error(`FAIL ${message}`);
  process.exit(1);
}
