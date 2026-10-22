/**
 * One-shot headed PoC: reuse profile, open Studio Content, download own video.
 * If login needed, waits for manual auth in the same window.
 */
import fs from 'node:fs';
import path from 'node:path';
import {chromium} from 'playwright';

const videoID = process.argv.find((a) => a.startsWith('--video-id='))?.slice(11) || 'KgUPTeDEf_o';
const outDir = path.resolve(process.argv.find((a) => a.startsWith('--out-dir='))?.slice(10) || '../tmp/studio-out');
const profileDir = path.resolve(process.argv.find((a) => a.startsWith('--profile-dir='))?.slice(14) || './.studio-profile');

fs.mkdirSync(outDir, {recursive: true});
fs.mkdirSync(profileDir, {recursive: true});

const log = (m) => console.error(JSON.stringify({type: 'log', message: m}));

const context = await chromium.launchPersistentContext(profileDir, {
  headless: false,
  acceptDownloads: true,
  viewport: {width: 1500, height: 960},
  args: ['--disable-blink-features=AutomationControlled'],
});
const page = context.pages()[0] || (await context.newPage());

async function ensureLoggedIn() {
  await page.goto('https://studio.youtube.com/', {waitUntil: 'domcontentloaded', timeout: 120000});
  const deadline = Date.now() + 5 * 60_000;
  while (Date.now() < deadline) {
    const url = page.url();
    log(`url=${url}`);
    if (url.includes('studio.youtube.com') && !url.includes('accounts.google.com')) {
      await page.waitForTimeout(2000);
      const hasChrome = (await page.locator('#avatar-btn, ytcp-topbar-menu-avatar, #create-icon').count()) > 0;
      if (hasChrome || url.includes('/channel/')) {
        log('authenticated');
        return true;
      }
    }
    log('waiting for manual login…');
    await page.waitForTimeout(2500);
  }
  return false;
}

if (!(await ensureLoggedIn())) {
  console.log(JSON.stringify({ok: false, error: 'login_timeout'}));
  await context.close();
  process.exit(1);
}

// Content / Videos list
await page.goto('https://studio.youtube.com/', {waitUntil: 'domcontentloaded'});
await page.waitForTimeout(1500);
const contentLink = page.locator('a#menu-item-1, a[href*="/videos/upload"], tp-yt-paper-item:has-text("Content"), tp-yt-paper-item:has-text("Контент")').first();
if ((await contentLink.count()) > 0) {
  await contentLink.click({timeout: 5000}).catch(() => {});
  await page.waitForTimeout(2500);
}
log(`content url=${page.url()}`);

// Search box if present
const search = page.locator('input#textbox, input[aria-label*="Search"], input[placeholder*="Search"], input[aria-label*="Поиск"]').first();
if ((await search.count()) > 0) {
  await search.fill(videoID).catch(() => {});
  await page.keyboard.press('Enter').catch(() => {});
  await page.waitForTimeout(2500);
  log('searched video id');
}

// Dump actionable controls for diagnostics
const dump = await page.evaluate((id) => {
  const all = [...document.querySelectorAll('a, button, ytcp-icon-button, tp-yt-paper-item')];
  return {
    hrefs: all
      .filter((el) => (el.getAttribute('href') || '').includes(id))
      .map((el) => el.getAttribute('href')),
    labels: all
      .map((el) => `${el.tagName}|${el.id}|${el.getAttribute('aria-label') || ''}|${(el.innerText || '').slice(0, 60)}`)
      .filter((t) => /download|скач|option|more|меню|overflow|action/i.test(t))
      .slice(0, 40),
  };
}, videoID);
fs.writeFileSync(path.join(outDir, 'dom-dump.json'), JSON.stringify(dump, null, 2));
log(`dump hrefs=${dump.hrefs.length} labels=${dump.labels.length}`);

// Try open video edit then look for download in page HTML via keyboard/menu
await page.goto(`https://studio.youtube.com/video/${videoID}/edit`, {waitUntil: 'domcontentloaded', timeout: 120000});
await page.waitForTimeout(4000);
log(`edit url=${page.url()}`);

const editDump = await page.evaluate(() => {
  return [...document.querySelectorAll('button, ytcp-icon-button, ytcp-button, a')]
    .map((el) => ({
      tag: el.tagName,
      id: el.id,
      aria: el.getAttribute('aria-label') || '',
      text: (el.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 80),
    }))
    .filter((x) => x.aria || x.text)
    .slice(0, 120);
});
fs.writeFileSync(path.join(outDir, 'edit-dump.json'), JSON.stringify({url: page.url(), editDump}, null, 2));

// Click any promising overflow
for (const sel of [
  'button[aria-label="Options"]',
  'button[aria-label="Опции"]',
  'button[aria-label="More actions"]',
  'button[aria-label="More options"]',
  'button[aria-label="Дополнительные действия"]',
  'ytcp-icon-button[id*="overflow"]',
  '#overflow-menu-button',
  'ytcp-button#overflow-menu-button',
]) {
  const n = page.locator(sel);
  if ((await n.count()) > 0) {
    await n.first().click({timeout: 3000}).catch(() => {});
    log(`clicked ${sel}`);
    await page.waitForTimeout(1000);
  }
}

// Also try top-right kebab via role
const kebabs = page.locator('[aria-haspopup="menu"], button[aria-label*="menu" i], button[aria-label*="Menu" i]');
const kc = await kebabs.count();
log(`kebab-like count=${kc}`);
for (let i = 0; i < Math.min(kc, 8); i++) {
  const label = await kebabs.nth(i).getAttribute('aria-label').catch(() => '');
  log(`kebab[${i}]=${label}`);
}

const menuTexts = await page.locator('tp-yt-paper-item, [role="menuitem"], ytcp-text-menu').allInnerTexts().catch(() => []);
log(`menus=${JSON.stringify(menuTexts).slice(0, 500)}`);

// Attempt download click if visible
const dl = page.locator('tp-yt-paper-item:has-text("Download"), tp-yt-paper-item:has-text("Скачать"), [role="menuitem"]:has-text("Download"), [role="menuitem"]:has-text("Скачать")');
if ((await dl.count()) > 0) {
  const [download] = await Promise.all([
    page.waitForEvent('download', {timeout: 120000}),
    dl.first().click(),
  ]);
  const dest = path.join(outDir, download.suggestedFilename() || `${videoID}.mp4`);
  await download.saveAs(dest);
  const st = fs.statSync(dest);
  console.log(JSON.stringify({ok: true, path: dest, bytes: st.size}));
  await context.close();
  process.exit(0);
}

console.log(
  JSON.stringify({
    ok: false,
    error: 'download_not_found',
    message: 'см. edit-dump.json / dom-dump.json для селекторов',
    editUrl: page.url(),
  }),
);
await context.close();
process.exit(1);
