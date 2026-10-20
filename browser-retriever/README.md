# browser-retriever (Playwright + SSYouTube UI)

Экспериментальный worker для A-Tex InfiDrive. Автоматизирует **веб-интерфейс** [SSYouTube](https://ssyoutube.com/), не API.

## Locators (исследование DOM 2026-10-20)

| Элемент | Locator |
|---------|---------|
| URL input | `#id_url` (`name=url`, placeholder «Paste your video link here») |
| Submit | `#search` (текст «Download») |
| Results | `#main-results` / `#convert-result` |
| MP4 + audio | `a[id^="download-mp4-"][id$="-audio"]:not([id*="no-audio"])` |
| CAPTCHA | `.captcha-dialog__document` / `img[alt="Captcha"]` — **не обходим** |

Путь `ssyoutube.com/watch?v=…` часто редиректит на SaveFrom.net с CAPTCHA — worker завершается ошибкой `captcha_required`.

## Установка

```bash
cd browser-retriever
npm install
# postinstall ставит Chromium для Playwright
```

## CLI

```bash
node src/cli.mjs --video-id=jNQXAC9IVRw --out-dir=./tmp-out --timeout-ms=120000
```

- stdout: один JSON `{ok, path, bytes, quality, error, …}`
- stderr: JSON lines `{type:"progress"|log,…}`

Go передаёт **только video_id** из SQLite (никаких пользовательских URL).

## VPS

```bash
# tmpfs RAM-диск
sudo mkdir -p /dev/shm/infidrive
# .env
BROWSER_RETRIEVER_TEMP=/dev/shm/infidrive
BROWSER_RETRIEVER_JS=/opt/infidrive/browser-retriever/src/cli.mjs
```

Chromium и Playwright создают свои temp-файлы — каталог на tmpfs ограничивает запись на диск.
