import fs from 'node:fs';
import path from 'node:path';
import {chromium} from 'playwright';
import {SS_HOME, isValidVideoID, locators, youtubeWatchURL} from './dom.mjs';

/**
 * @typedef {{stage: string, pct: number, message?: string}} Progress
 * @typedef {{
 *   ok: boolean,
 *   path?: string,
 *   bytes?: number,
 *   quality?: string,
 *   format?: string,
 *   duration_ms?: number,
 *   error?: string,
 *   message?: string,
 *   logs?: string[],
 * }} Result
 */

function emitProgress(onProgress, stage, pct, message) {
  const ev = {stage, pct, message};
  if (typeof onProgress === 'function') {
    onProgress(ev);
  }
  // Machine-readable progress for Go parent (stderr).
  console.error(JSON.stringify({type: 'progress', ...ev}));
}

function logLine(logs, msg) {
  const line = `[browser-retriever] ${msg}`;
  logs.push(line);
  console.error(JSON.stringify({type: 'log', message: msg}));
}

/**
 * Download MP4 for a known YouTube video ID via SSYouTube UI.
 * Never accepts arbitrary user URLs — only videoID → constructed watch URL.
 *
 * @param {{videoID: string, outDir: string, timeoutMs?: number, headless?: boolean, onProgress?: (p: Progress)=>void}} opts
 * @returns {Promise<Result>}
 */
export async function retrieveMP4(opts) {
  const started = Date.now();
  const logs = [];
  const videoID = String(opts.videoID || '').trim();
  const outDir = opts.outDir;
  const timeoutMs = opts.timeoutMs ?? 120_000;
  const headless = opts.headless !== false;
  const onProgress = opts.onProgress;

  if (!isValidVideoID(videoID)) {
    return {ok: false, error: 'invalid_video_id', message: 'некорректный video_id', logs};
  }
  if (!outDir) {
    return {ok: false, error: 'no_out_dir', message: 'не задан каталог выгрузки', logs};
  }
  fs.mkdirSync(outDir, {recursive: true});

  const watchURL = youtubeWatchURL(videoID);
  logLine(logs, `start video_id=${videoID} headless=${headless}`);
  emitProgress(onProgress, 'launch', 5, 'Запуск браузера');

  let browser;
  try {
    browser = await chromium.launch({
      headless,
      args: [
        '--disable-dev-shm-usage',
        '--no-sandbox',
        '--disable-gpu',
        '--memory-pressure-off',
      ],
    });
  } catch (err) {
    return {
      ok: false,
      error: 'browser_launch_failed',
      message: String(err?.message || err),
      logs,
      duration_ms: Date.now() - started,
    };
  }

  const context = await browser.newContext({
    acceptDownloads: true,
    locale: 'en-US',
    userAgent:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    viewport: {width: 1280, height: 800},
  });
  context.setDefaultTimeout(Math.min(timeoutMs, 60_000));

  const page = await context.newPage();
  page.on('dialog', async (d) => {
    logLine(logs, `dismiss dialog: ${d.type()} ${d.message()}`);
    await d.dismiss().catch(() => {});
  });

  const deadline = Date.now() + timeoutMs;
  const remaining = () => Math.max(1000, deadline - Date.now());

  try {
    emitProgress(onProgress, 'open_site', 15, 'Открытие страницы');
    await page.goto(SS_HOME, {waitUntil: 'domcontentloaded', timeout: remaining()});
    logLine(logs, `opened ${page.url()}`);

    // Dismiss extension upsell if present (do not install).
    await dismissOverlays(page, logs);

    if (await page.locator(locators.captchaDialog).first().isVisible().catch(() => false)) {
      return failCaptcha(logs, started);
    }

    emitProgress(onProgress, 'submit_url', 30, 'Подготовка файла');
    const input = page.locator(locators.urlInput);
    await input.waitFor({state: 'visible', timeout: remaining()});
    await input.fill('');
    await input.fill(watchURL);
    logLine(logs, `filled url input`);

    await page.locator(locators.submit).click({timeout: remaining()});
    logLine(logs, 'clicked Download');

    // Redirect to savefrom / captcha is a hard stop.
    await page.waitForTimeout(1500);
    await dismissOverlays(page, logs);
    if (await detectCaptcha(page)) {
      return failCaptcha(logs, started);
    }
    if (/savefrom\.net/i.test(page.url())) {
      logLine(logs, `redirected to ${page.url()}`);
      if (await detectCaptcha(page)) {
        return failCaptcha(logs, started);
      }
      // SaveFrom path often requires CAPTCHA/subscription; fail honestly if no MP4 controls.
      const hasMp4 = await page.locator(locators.mp4WithAudio).first().isVisible().catch(() => false);
      if (!hasMp4 && (await detectCaptcha(page))) {
        return failCaptcha(logs, started);
      }
    }

    emitProgress(onProgress, 'wait_formats', 50, 'Ожидание вариантов файла');
    const results = page.locator(locators.convertResult).or(page.locator(locators.results));
    try {
      await results.first().waitFor({state: 'visible', timeout: remaining()});
    } catch {
      if (await detectCaptcha(page)) {
        return failCaptcha(logs, started);
      }
      return {
        ok: false,
        error: 'formats_timeout',
        message: 'варианты скачивания не появились вовремя',
        logs,
        duration_ms: Date.now() - started,
      };
    }

    // Expand list if present.
    const more = page.locator(locators.showMore);
    if (await more.isVisible().catch(() => false)) {
      await more.click().catch(() => {});
      await page.waitForTimeout(400);
    }

    let target = page.locator(locators.mp4WithAudio).first();
    if (!(await target.isVisible().catch(() => false))) {
      target = page.locator(locators.mp4Any).first();
    }
    if (!(await target.isVisible().catch(() => false))) {
      if (await detectCaptcha(page)) {
        return failCaptcha(logs, started);
      }
      return {
        ok: false,
        error: 'no_mp4_option',
        message: 'нет доступного варианта MP4 с видеодорожкой',
        logs,
        duration_ms: Date.now() - started,
      };
    }

    const qualityId = (await target.getAttribute('id')) || '';
    const qualityMatch = /download-mp4-(\d+)/.exec(qualityId);
    const quality = qualityMatch ? qualityMatch[1] : 'unknown';
    logLine(logs, `selected ${qualityId}`);

    emitProgress(onProgress, 'download', 70, 'Загрузка файла');
    const downloadPromise = page.waitForEvent('download', {timeout: remaining()});
    await target.click({timeout: remaining()});

    // New tab / navigation edge cases.
    const popupPromise = context.waitForEvent('page', {timeout: 3000}).catch(() => null);
    const popup = await popupPromise;
    if (popup) {
      logLine(logs, `new tab ${popup.url()}`);
      if (await detectCaptcha(popup)) {
        await popup.close().catch(() => {});
        return failCaptcha(logs, started);
      }
    }

    let download;
    try {
      download = await downloadPromise;
    } catch (err) {
      if (await detectCaptcha(page) || (popup && (await detectCaptcha(popup)))) {
        return failCaptcha(logs, started);
      }
      return {
        ok: false,
        error: 'download_timeout',
        message: `событие download не получено: ${err?.message || err}`,
        logs,
        duration_ms: Date.now() - started,
      };
    }

    const suggested = download.suggestedFilename() || `${videoID}.mp4`;
    const outPath = path.join(outDir, sanitizeName(suggested.endsWith('.mp4') ? suggested : `${videoID}.mp4`));
    await download.saveAs(outPath);
    const st = fs.statSync(outPath);
    if (!st.size) {
      return {
        ok: false,
        error: 'empty_file',
        message: 'скачанный файл пуст',
        logs,
        duration_ms: Date.now() - started,
      };
    }

    emitProgress(onProgress, 'done', 100, 'Файл получен');
    logLine(logs, `saved ${outPath} bytes=${st.size}`);
    return {
      ok: true,
      path: outPath,
      bytes: st.size,
      quality,
      format: 'mp4',
      duration_ms: Date.now() - started,
      logs,
    };
  } catch (err) {
    const msg = String(err?.message || err);
    logLine(logs, `error: ${msg}`);
    if (/captcha/i.test(msg) || (await detectCaptcha(page).catch(() => false))) {
      return failCaptcha(logs, started);
    }
    return {
      ok: false,
      error: 'unexpected',
      message: msg,
      logs,
      duration_ms: Date.now() - started,
    };
  } finally {
    await context.close().catch(() => {});
    await browser.close().catch(() => {});
    logLine(logs, 'browser closed');
  }
}

async function dismissOverlays(page, logs) {
  try {
    const close = page.locator(locators.extensionClose).first();
    if (await close.isVisible({timeout: 800}).catch(() => false)) {
      await close.click({timeout: 1000}).catch(() => {});
      logLine(logs, 'dismissed overlay');
    }
  } catch {
    /* ignore */
  }
  await page.keyboard.press('Escape').catch(() => {});
}

async function detectCaptcha(page) {
  const captcha = page.locator(locators.captchaDialog).first();
  if (await captcha.isVisible().catch(() => false)) {
    return true;
  }
  const text = await page.locator('body').innerText().catch(() => '');
  return /enter captcha|captcha/i.test(text) && /submit/i.test(text);
}

function failCaptcha(logs, started) {
  logLine(logs, 'CAPTCHA detected — aborting without bypass');
  return {
    ok: false,
    error: 'captcha_required',
    message: 'SSYouTube/SaveFrom запросил CAPTCHA; автоматизация остановлена без обхода',
    logs,
    duration_ms: Date.now() - started,
  };
}

function sanitizeName(name) {
  return String(name).replace(/[^\w.\-()+ ]+/g, '_').slice(0, 120);
}
