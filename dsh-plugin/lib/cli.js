/**
 * The LNReaderTUI command line, driven as a child process.
 *
 * Everything that talks to bilinovel.com — search, detail, download, update,
 * import — is delegated to the installed `lnreadertui` binary so the panel
 * gets exactly the terminal tool's scraping, rate limiting, EPUB assembly and
 * library registration. Long operations run as cancellable jobs whose progress
 * lines the CLI already prints on stderr.
 */

import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { access, constants } from 'node:fs/promises';
import { homedir } from 'node:os';
import { delimiter, join } from 'node:path';

const PROBE_TIMEOUT_MS = 8000;
const MAX_LOG_LINES = 200;

/** Directories worth checking when the binary is not on PATH. */
function fallbackPaths() {
  const names = process.platform === 'win32' ? ['lnreadertui.exe'] : ['lnreadertui'];
  const directories = [
    process.env.LNREADERTUI_BIN_DIR,
    process.platform === 'win32' ? process.env.LOCALAPPDATA : undefined,
    join(homedir(), '.linuxbrew', 'bin'),
    join(homedir(), '.local', 'bin'),
    join(homedir(), 'go', 'bin'),
    '/opt/homebrew/bin',
    '/home/linuxbrew/.linuxbrew/bin',
    '/usr/local/bin',
    '/usr/bin',
  ].filter((directory) => typeof directory === 'string' && directory !== '');
  const paths = [];
  for (const directory of directories) {
    for (const name of names) paths.push(join(directory, name));
  }
  return paths;
}

/**
 * Create the CLI driver for one plugin instance.
 * @param {{dataDir: string, bin?: string}} options
 */
export function createCli(options) {
  const configured = [options.bin, process.env.LNREADERTUI_BIN].filter((value) => typeof value === 'string' && value.trim() !== '');
  let cached = null;

  /**
   * Resolve the executable once: an explicit path, then PATH, then the usual
   * install directories. The answer is cached for the plugin's lifetime.
   * @returns {Promise<string>}
   */
  async function resolve() {
    if (cached !== null) return cached;
    const candidates = [...configured, 'lnreadertui', ...fallbackPaths()];
    const tried = [];
    for (const candidate of candidates) {
      if (candidate.includes('/') || candidate.includes('\\')) {
        try {
          await access(candidate, constants.X_OK);
        } catch {
          tried.push(candidate);
          continue;
        }
      }
      const ok = await probe(candidate);
      if (ok) {
        cached = candidate;
        return candidate;
      }
      tried.push(candidate);
    }
    const error = new Error(
      'lnreadertui executable not found. Install LNReaderTUI (brew install hhdtc/tap/lnreadertui, '
      + 'winget install hhdtc.LNReaderTUI, or a release binary) and put it on PATH, or set '
      + `LNREADERTUI_BIN. Tried: ${tried.join(', ') || '(none)'}`,
    );
    error.code = 'CLI_MISSING';
    throw error;
  }

  /** Report whether the CLI is usable without throwing. */
  async function status() {
    try {
      const path = await resolve();
      return { available: true, path, version: await version(path) };
    } catch (error) {
      return { available: false, path: null, error: error instanceof Error ? error.message : String(error) };
    }
  }

  /**
   * Run one CLI command and collect its output.
   * @param {string[]} args
   * @param {{timeoutMs?: number, parseJson?: boolean}} [runOptions]
   * @returns {Promise<{json: any, stdout: string, stderr: string, code: number}>}
   */
  async function run(args, runOptions = {}) {
    const bin = await resolve();
    const timeoutMs = runOptions.timeoutMs ?? 120000;
    return await new Promise((settle, fail) => {
      const child = spawn(bin, ['-data-dir', options.dataDir, ...args], {
        cwd: options.dataDir,
        env: childEnvironment(),
        windowsHide: true,
      });
      let stdout = '';
      let stderr = '';
      let timedOut = false;
      const timer = setTimeout(() => {
        timedOut = true;
        child.kill('SIGTERM');
      }, timeoutMs);
      child.stdout.setEncoding('utf8');
      child.stderr.setEncoding('utf8');
      child.stdout.on('data', (chunk) => { stdout += chunk; });
      child.stderr.on('data', (chunk) => { stderr += chunk; });
      child.on('error', (error) => {
        clearTimeout(timer);
        fail(error);
      });
      child.on('close', (code) => {
        clearTimeout(timer);
        if (timedOut) {
          fail(new Error(`lnreadertui timed out after ${timeoutMs}ms`));
          return;
        }
        if (code !== 0) {
          const reason = stderr.trim() || stdout.trim() || `exit code ${code}`;
          const error = new Error(reason.split('\n').slice(-6).join('\n'));
          error.code = 'CLI_FAILED';
          error.exitCode = code;
          error.stdout = stdout;
          error.stderr = stderr;
          fail(error);
          return;
        }
        settle({ json: runOptions.parseJson === false ? null : lastJsonObject(stdout), stdout, stderr, code });
      });
    });
  }

  /**
   * Start a long-running command as an observable job.
   * @param {{kind: string, args: string[], label?: string}} request
   * @returns {Promise<any>} the job record.
   */
  async function startJob(request) {
    const bin = await resolve();
    const job = {
      id: randomBytes(6).toString('hex'),
      kind: request.kind,
      label: request.label ?? request.kind,
      args: request.args,
      status: 'running',
      startedAt: new Date().toISOString(),
      endedAt: null,
      exitCode: null,
      progress: { done: 0, total: 0, volume: '', title: '' },
      logs: [],
      result: null,
      error: null,
      child: null,
      cancelRequested: false,
    };
    const child = spawn(bin, ['-data-dir', options.dataDir, ...request.args], {
      cwd: options.dataDir,
      env: childEnvironment(),
      windowsHide: true,
    });
    job.child = child;
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
      for (const line of lines(chunk)) pushLog(job, 'out', line);
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
      for (const line of lines(chunk)) {
        const progress = parseProgress(line);
        if (progress !== null) job.progress = progress;
        else pushLog(job, 'err', line);
      }
    });
    child.on('error', (error) => {
      job.status = 'failed';
      job.error = error.message;
      job.endedAt = new Date().toISOString();
      job.child = null;
    });
    child.on('close', (code) => {
      job.exitCode = code;
      job.endedAt = new Date().toISOString();
      job.child = null;
      if (job.cancelRequested) {
        job.status = 'cancelled';
        job.error = null;
        return;
      }
      if (code === 0) {
        job.status = 'done';
        job.result = lastJsonObject(stdout);
        const done = job.progress.total;
        if (done > 0) job.progress = { ...job.progress, done, title: job.progress.title };
        return;
      }
      job.status = 'failed';
      job.error = (stderr.trim() || stdout.trim() || `exit code ${code}`).split('\n').slice(-6).join('\n');
    });
    return job;
  }

  /**
   * Ask a running job to stop: SIGINT first (the CLI cancels cleanly), SIGKILL
   * if it is still alive a few seconds later.
   * @param {any} job
   * @param {number} [graceMs]
   */
  function cancelJob(job, graceMs = 6000) {
    if (job === null || job.child === null) return false;
    job.cancelRequested = true;
    const child = job.child;
    try {
      child.kill('SIGINT');
    } catch {
      return false;
    }
    const timer = setTimeout(() => {
      if (job.child === child) {
        try {
          child.kill('SIGKILL');
        } catch { /* already gone */ }
      }
    }, graceMs);
    timer.unref?.();
    return true;
  }

  return { resolve, status, run, startJob, cancelJob, dataDir: options.dataDir };
}

function childEnvironment() {
  const env = { ...process.env };
  // The CLI writes UTF-8 progress; keep that promise on Windows consoles.
  env.PYTHONIOENCODING = 'utf-8';
  return env;
}

async function probe(bin) {
  return await new Promise((settle) => {
    let child;
    try {
      child = spawn(bin, ['--version'], { windowsHide: true, env: childEnvironment() });
    } catch {
      settle(false);
      return;
    }
    const timer = setTimeout(() => {
      try {
        child.kill('SIGKILL');
      } catch { /* ignore */ }
      settle(false);
    }, PROBE_TIMEOUT_MS);
    child.on('error', () => {
      clearTimeout(timer);
      settle(false);
    });
    child.on('close', () => {
      clearTimeout(timer);
      settle(true);
    });
  });
}

async function version(bin) {
  return await new Promise((settle) => {
    let child;
    try {
      child = spawn(bin, ['--version'], { windowsHide: true, env: childEnvironment() });
    } catch {
      settle(null);
      return;
    }
    let output = '';
    const timer = setTimeout(() => {
      try {
        child.kill('SIGKILL');
      } catch { /* ignore */ }
      settle(null);
    }, PROBE_TIMEOUT_MS);
    child.stdout?.setEncoding('utf8');
    child.stdout?.on('data', (chunk) => { output += chunk; });
    child.on('error', () => {
      clearTimeout(timer);
      settle(null);
    });
    child.on('close', () => {
      clearTimeout(timer);
      settle(output.trim() || null);
    });
  });
}

/** Split a stderr chunk into carriage-return separated progress lines. */
function lines(chunk) {
  return chunk.split(/[\r\n]+/).map((line) => line.trim()).filter((line) => line !== '');
}

/**
 * Read one CLI progress line: `[ 12/85] 第二卷 · Case-03` or `title…`.
 * @param {string} line
 * @returns {{done: number, total: number, volume: string, title: string} | null}
 */
export function parseProgress(line) {
  const counted = /^\[\s*(\d+)\/(\d+)\]\s*(.*)$/.exec(line);
  if (counted !== null) {
    const title = counted[3].trim();
    const separator = title.indexOf(' · ');
    return {
      done: Number(counted[1]),
      total: Number(counted[2]),
      volume: separator < 0 ? '' : title.slice(0, separator).trim(),
      title: separator < 0 ? title : title.slice(separator + 3).trim(),
    };
  }
  const pending = /^(.*)…$/.exec(line);
  if (pending !== null && line.length < 200) {
    return { done: 0, total: 0, volume: '', title: pending[1].trim() };
  }
  return null;
}

function pushLog(job, stream, text) {
  job.logs.push({ at: new Date().toISOString(), stream, text });
  if (job.logs.length > MAX_LOG_LINES) job.logs.splice(0, job.logs.length - MAX_LOG_LINES);
}

/**
 * The last complete JSON object printed on stdout, if any (the CLI prints one
 * result object with `--json`).
 * @param {string} stdout
 * @returns {any | null}
 */
export function lastJsonObject(stdout) {
  const text = stdout.trim();
  if (text === '') return null;
  for (const candidate of [text, text.slice(text.indexOf('{')), text.slice(text.lastIndexOf('{'))]) {
    if (candidate === '') continue;
    try {
      return JSON.parse(candidate);
    } catch { /* try the next candidate */ }
  }
  return null;
}

/** Environment PATH delimiter, exported for diagnostics. */
export const PATH_DELIMITER = delimiter;
