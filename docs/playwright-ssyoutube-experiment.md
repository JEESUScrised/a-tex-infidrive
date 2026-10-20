# Playwright + SSYouTube Recovery Experiment

Учебный эксперимент A-Tex InfiDrive. **Не production.**

## DOM (живое исследование 2026-10-20)

Стартовая страница: `https://en.ssyoutube.com/1ab/`

| UI | Locator |
|----|---------|
| Поле ссылки | `#id_url` |
| Кнопка | `#search` («Download») |
| Блок результатов | `#convert-result` внутри `#main-results` |
| MP4 с аудио | `#download-mp4-{quality}-audio` (например `240`) |
| MP4 без звука | `#download-mp4-{quality}-no-audio` |
| CAPTCHA (SaveFrom) | `.captcha-dialog__document` — **не обходим** |

Предпочтительный выбор: первая ссылка `a[id^="download-mp4-"][id$="-audio"]:not([id*="no-audio"])`.

## Результат прогона Playwright (публичный ролик)

Тест worker: `node src/cli.mjs --video-id=jNQXAC9IVRw --out-dir=tmp-out`

| Параметр | Значение |
|----------|----------|
| Video ID | `jNQXAC9IVRw` (Me at the zoo — публичный smoke-тест UI) |
| Удалось скачать | **да** |
| Формат | MP4 |
| Quality UI | 240 (с audio) |
| Размер файла | 629 172 байт |
| Время worker | ≈ 10.7 с (`duration_ms=10698`) |
| ffprobe | см. ниже |
| SHA-256 payload | **не применимо** к чужому публичному ролику (это smoke UI, не наш encrypted payload) |

### ffprobe (скачанный файл)

| Поле | Значение |
|------|----------|
| format_name | `mov,mp4,m4a,3gp,3g2,mj2` |
| duration | ≈ 18.95 с |
| video | H.264 320×240 |
| audio | AAC |
| читаемость | да |

## Полный цикл SHA (собственный public YouTube)

Требует OAuth + `EXPERIMENTAL_YOUTUBE_STORE` + публичный тестовый ролик с AES-GCM payload.

1. Upload encrypt→encode→YouTube public  
2. Playwright retrieve скачанного варианта (не локального MP4)  
3. Decode → decrypt → SHA  

Статус в этой сессии: **smoke download доказан; live SHA — после credentials**.

## VPS

```bash
sudo mkdir -p /dev/shm/infidrive
# .env
BROWSER_RETRIEVER_JS=/opt/infidrive/browser-retriever/src/cli.mjs
BROWSER_RETRIEVER_TEMP=/dev/shm/infidrive
BROWSER_RETRIEVER_HEADLESS=true
```

Нужны: Node 18+, `npm install` + `npm run install-browser` в `browser-retriever/`, Chromium deps Linux.

Ограничения памяти: headless Chromium + tmpfs; после операции каталог temp удаляется Go-адаптером.

## Ошибки, которые считаются честным провалом

- `captcha_required` — CAPTCHA/антибот  
- `formats_timeout` / `no_mp4_option` — нет вариантов  
- `download_timeout` — нет Playwright download event  

Фиктивный успех запрещён.
