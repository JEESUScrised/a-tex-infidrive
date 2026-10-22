import fs from 'node:fs';
import path from 'node:path';
import {chromium} from 'playwright';
import {isValidVideoID} from './dom.mjs';

/**
 * YouTube Studio owner-download PoC.
 * Uses official Studio "Download" for the channel owner's own videos only.
 * First Google login is manual (headed); no password automation / CAPTCHA bypass.
 */

const STUDIO_HOME = 'https://studio.youtube.com/';

function emitProgress(stage, pct, message) {
  console.error(JSON.stringify({type: 'progress', stage, pct, message}));
}

function logLine(logs, msg) {
  logs.push(`[studio-retriever] ${msg}`);
  console.error(JSON.stringify({type: 'log', message: msg}));
}

function looksLikeLogin(url) {
  const u = String(url || '').toLowerCase();
  // Avoid false positives on studio.youtube.com paths.
  if (u.includes('studio.youtube.com') && !u.includes('accounts.google.com')) {
    return false;
  }
  return (
    u.includes('accounts.google.com') ||
    u.includes('accounts.youtube.com') ||
    u.includes('/signin') ||
    u.includes('/serviceLogin'.toLowerCase()) ||
    u.includes('/challenge')
  );
}

async function isStudioAuthenticated(page) {
  const url = page.url();
  if (looksLikeLogin(url)) return false;
  if (!url.includes('studio.youtube.com')) return false;
  // Logged-in Studio usually shows create / avatar / dashboard chrome.
  const markers = [
    '#avatar-btn',
    'button[aria-label*="Create"]',
    'button[aria-label*="Создать"]',
    '#create-icon',
    'ytcp-topbar-menu-avatar',
    'ytcp-animatable#content-wrapper',
  ];
  for (const sel of markers) {
    if ((await page.locator(sel).count()) > 0) return true;
  }
  // Fallback: not on accounts and body mentions Dashboard/Content.
  const body = await page.locator('body').innerText({timeout: 3000}).catch(() => '');
  if (/dashboard|content|контент|канал|channel/i.test(body) && !/sign in|войти/i.test(body.slice(0, 500))) {
    return true;
  }
  return false;
}

function looksLikeBlocked(url, bodyText) {
  const t = `${url}\n${bodyText || ''}`.toLowerCase();
  return (
    t.includes('unusual traffic') ||
    t.includes('automated queries') ||
    t.includes('detected unusual') ||
    t.includes('recaptcha') ||
    t.includes('try again later')
  );
}

/**
 * Interactive login helper: opens headed Chromium with persistent profile and waits
 * until Studio is reachable without accounts.google.com redirect.
 */
export async function loginStudio(opts) {
  const logs = [];
  const profileDir = opts.profileDir;
  const timeoutMs = opts.timeoutMs ?? 10 * 60_000;
  if (!profileDir) {
    return {ok: false, error: 'no_profile', message: 'нужен --profile-dir', logs};
  }
  fs.mkdirSync(profileDir, {recursive: true});
  logLine(logs, `login profile=${profileDir}`);
  emitProgress('login', 5, 'Откройте окно Chromium и войдите в Google / YouTube Studio вручную');

  const context = await chromium.launchPersistentContext(profileDir, {
    headless: false,
    acceptDownloads: true,
    viewport: {width: 1400, height: 900},
    args: ['--disable-blink-features=AutomationControlled'],
  });
  const page = context.pages()[0] || (await context.newPage());
  await page.goto(STUDIO_HOME, {waitUntil: 'domcontentloaded', timeout: 120_000});

  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const url = page.url();
    if (looksLikeLogin(url)) {
      emitProgress('login', 20, 'Ожидание ручного входа (включая 2FA)…');
    } else if (await isStudioAuthenticated(page)) {
      const body = await page.locator('body').innerText({timeout: 5000}).catch(() => '');
      if (looksLikeBlocked(url, body)) {
        await context.close();
        return {
          ok: false,
          error: 'automation_blocked',
          message: 'Google ограничил доступ (подозрение на автоматизацию). Ручной сценарий Studio сохраняется.',
          logs,
        };
      }
      // Flush storage before close.
      await page.goto(STUDIO_HOME, {waitUntil: 'domcontentloaded', timeout: 60_000}).catch(() => {});
      await page.waitForTimeout(2000);
      emitProgress('login', 100, 'Сессия Studio сохранена в профиле');
      await context.close();
      return {ok: true, profileDir, logs, message: 'вход выполнен, профиль сохранён'};
    }
    await page.waitForTimeout(2000);
  }
  await context.close();
  return {ok: false, error: 'login_timeout', message: 'время ожидания ручного входа истекло', logs};
}

/**
 * Download own video MP4 via Studio UI.
 * @returns {Promise<object>}
 */
export async function retrieveStudioMP4(opts) {
  const started = Date.now();
  const logs = [];
  const videoID = String(opts.videoID || '').trim();
  const outDir = opts.outDir;
  const profileDir = opts.profileDir;
  const timeoutMs = opts.timeoutMs ?? 180_000;
  const headless = opts.headless !== false;

  if (!isValidVideoID(videoID)) {
    return {ok: false, error: 'invalid_video_id', message: 'некорректный video_id', logs};
  }
  if (!outDir || !profileDir) {
    return {ok: false, error: 'bad_args', message: 'нужны outDir и profileDir', logs};
  }
  fs.mkdirSync(outDir, {recursive: true});
  fs.mkdirSync(profileDir, {recursive: true});

  logLine(logs, `start video_id=${videoID} headless=${headless}`);
  emitProgress('launch', 5, 'Запуск Chromium (Studio profile)');

  let context;
  try {
    context = await chromium.launchPersistentContext(profileDir, {
      headless,
      acceptDownloads: true,
      viewport: {width: 1400, height: 900},
      args: ['--disable-blink-features=AutomationControlled'],
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

  const page = context.pages()[0] || (await context.newPage());
  try {
    emitProgress('open_studio', 15, 'Открытие YouTube Studio');
    await page.goto(STUDIO_HOME, {waitUntil: 'domcontentloaded', timeout: 120_000});
    await page.waitForTimeout(2500);
    logLine(logs, `studio home url=${page.url()}`);
    if (looksLikeLogin(page.url()) || !(await isStudioAuthenticated(page))) {
      await context.close();
      return {
        ok: false,
        error: 'login_required',
        message: `нет сохранённой сессии Studio (url=${page.url()}) — выполните: node src/studio-cli.mjs --login --profile-dir=...`,
        logs,
        duration_ms: Date.now() - started,
      };
    }
    const body0 = await page.locator('body').innerText({timeout: 8000}).catch(() => '');
    if (looksLikeBlocked(page.url(), body0)) {
      await context.close();
      return {
        ok: false,
        error: 'automation_blocked',
        message: 'Google ограничил автоматизацию. Используйте ручной Download из Studio.',
        logs,
        duration_ms: Date.now() - started,
      };
    }

    // Prefer direct video page (owner tools).
    const editURL = `https://studio.youtube.com/video/${videoID}/edit`;
    emitProgress('open_video', 30, 'Открытие страницы ролика в Studio');
    await page.goto(editURL, {waitUntil: 'domcontentloaded', timeout: 120_000});
    await page.waitForTimeout(2500);
    if (looksLikeLogin(page.url())) {
      await context.close();
      return {
        ok: false,
        error: 'login_required',
        message: 'сессия истекла — повторите --login',
        logs,
        duration_ms: Date.now() - started,
      };
    }
    logLine(logs, `opened ${page.url()}`);

    // If redirected away from this video (wrong channel / not found).
    if (!page.url().includes(videoID) && !page.url().includes('/video/')) {
      logLine(logs, `unexpected url after edit nav: ${page.url()}`);
    }

    emitProgress('menu', 45, 'Поиск действия «Скачать»');
    const downloaded = await clickDownloadAndSave(page, context, outDir, videoID, timeoutMs, logs);
    if (!downloaded.ok) {
      await context.close();
      return {...downloaded, logs, duration_ms: Date.now() - started};
    }

    await context.close();
    emitProgress('done', 100, 'MP4 получен из Studio');
    return {
      ok: true,
      path: downloaded.path,
      bytes: downloaded.bytes,
      quality: downloaded.quality || 'studio',
      format: 'mp4',
      width: downloaded.width,
      height: downloaded.height,
      logs,
      duration_ms: Date.now() - started,
    };
  } catch (err) {
    try {
      await context.close();
    } catch {
      /* ignore */
    }
    return {
      ok: false,
      error: 'studio_failed',
      message: String(err?.message || err),
      logs,
      duration_ms: Date.now() - started,
    };
  }
}

async function clickDownloadAndSave(page, context, outDir, videoID, timeoutMs, logs) {
  // Collect candidate clickables for Download / Скачать.
  // Observed working controls on RU Studio edit page (2026-10):
  // ytcp-icon-button[id*="overflow"] / #overflow-menu-button → menu contains «Скачать».
  const overflowSelectors = [
    'ytcp-icon-button[id*="overflow"]',
    '#overflow-menu-button',
    'ytcp-icon-button#overflow-menu-button',
    'button[aria-label*="More actions"]',
    'button[aria-label*="More options"]',
    'button[aria-label*="Options"]',
    'button[aria-label*="Опции"]',
    'button[aria-label*="Другие действия"]',
    'button[aria-label*="Дополнительные действия"]',
    'ytcp-button#overflow-button',
  ];

  let opened = false;
  for (const sel of overflowSelectors) {
    const loc = page.locator(sel);
    const n = await loc.count();
    for (let i = 0; i < Math.min(n, 3); i++) {
      try {
        await loc.nth(i).click({timeout: 2500});
        opened = true;
        logLine(logs, `clicked overflow ${sel} #${i}`);
        await page.waitForTimeout(600);
        // Stop once Download item appears.
        if ((await page.locator('tp-yt-paper-item:has-text("Скачать"), tp-yt-paper-item:has-text("Download")').count()) > 0) {
          break;
        }
      } catch {
        /* try next */
      }
    }
    if (opened && (await page.locator('tp-yt-paper-item:has-text("Скачать"), tp-yt-paper-item:has-text("Download")').count()) > 0) {
      break;
    }
  }

  // Fallback: Content list.
  if ((await page.locator('tp-yt-paper-item:has-text("Скачать"), tp-yt-paper-item:has-text("Download")').count()) === 0) {
    logLine(logs, 'overflow download missing — trying content list');
    await page.goto('https://studio.youtube.com/', {waitUntil: 'domcontentloaded', timeout: 120_000});
    await page.waitForTimeout(1500);
    const contentNav = page.locator('a[href*="/videos"], a[href*="/content"], tp-yt-paper-item:has-text("Content"), tp-yt-paper-item:has-text("Контент")').first();
    if ((await contentNav.count()) > 0) {
      await contentNav.click({timeout: 5000}).catch(() => {});
      await page.waitForTimeout(2000);
    }
    const row = page.locator(`a[href*="/video/${videoID}/"]`).first();
    if ((await row.count()) === 0) {
      return {
        ok: false,
        error: 'video_not_found',
        message: 'ролик не найден в Studio текущего аккаунта (другой канал?)',
      };
    }
    await row.hover().catch(() => {});
    const menuBtn = page
      .locator(
        `ytcp-video-row:has(a[href*="${videoID}"]) button, ytcp-video-row:has(a[href*="${videoID}"]) ytcp-icon-button`,
      )
      .last();
    if ((await menuBtn.count()) === 0) {
      return {ok: false, error: 'menu_not_found', message: 'не найдена кнопка меню ролика в Studio'};
    }
    await menuBtn.click({timeout: 5000});
    opened = true;
    logLine(logs, 'opened row action menu');
    await page.waitForTimeout(600);
  }

  await page.waitForTimeout(800);

  // Download menu item (EN/RU).
  const downloadItem = page
    .locator(
      [
        'tp-yt-paper-item:has-text("Download")',
        'tp-yt-paper-item:has-text("Скачать")',
        'ytcp-text-menu tp-yt-paper-item:has-text("Download")',
        'ytcp-text-menu tp-yt-paper-item:has-text("Скачать")',
        '[role="menuitem"]:has-text("Download")',
        '[role="menuitem"]:has-text("Скачать")',
        'text=Download',
        'text=Скачать',
      ].join(', '),
    )
    .first();

  if ((await downloadItem.count()) === 0) {
    const menuText = await page.locator('tp-yt-paper-listbox, ytcp-text-menu, [role="menu"]').innerText().catch(() => '');
    logLine(logs, `menu text: ${menuText.slice(0, 400)}`);
    if (/5 times|5 раз|limit|лимит/i.test(menuText)) {
      return {
        ok: false,
        error: 'download_limit',
        message: 'лимит Studio: не более 5 загрузок ролика за 24 часа',
      };
    }
    return {
      ok: false,
      error: 'download_action_missing',
      message: 'пункт «Скачать/Download» недоступен (права канала, страйк, preapproved audio, или лимит)',
    };
  }

  emitProgress('download', 70, 'Ожидание download event');
  const [download] = await Promise.all([
    page.waitForEvent('download', {timeout: Math.min(timeoutMs, 170_000)}),
    downloadItem.click({timeout: 5000}),
  ]);

  const suggested = download.suggestedFilename() || `${videoID}.mp4`;
  const dest = path.join(outDir, suggested.endsWith('.mp4') ? suggested : `${videoID}.mp4`);
  await download.saveAs(dest);
  const st = fs.statSync(dest);
  if (!st.size) {
    return {ok: false, error: 'empty_file', message: 'скачан пустой файл'};
  }
  logLine(logs, `saved ${dest} bytes=${st.size}`);
  return {ok: true, path: dest, bytes: st.size, quality: 'studio_owner_download'};
}
