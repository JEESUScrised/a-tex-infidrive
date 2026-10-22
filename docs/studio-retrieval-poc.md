# YouTube Studio Retrieval — применимость и PoC

## Техническая применимость

| Пункт | Оценка |
|-------|--------|
| Официальный Download своих роликов | Да ([YouTube Help](https://support.google.com/youtube/answer/56100)): MP4 **720p или 360p** |
| Лимит | ≤5 скачиваний одного ролика / 24 ч |
| Playwright + persistent profile | Да: ручной вход/2FA один раз, далее reuse cookies профиля |
| OAuth API tokens ≠ browser session | Соблюдаем: API tokens не подставляются в Chromium |
| SSYouTube | Снят с основного пути |

**Критическое ограничение кодека:** Studio отдаёт не исходный upload (даже если был 1080p). Codec v2 на сильном даунскейле ломается — это подтвердил PoC.

## Правовая / ToS применимость

- Скачивание **своего** контента через Studio — штатная функция владельца канала.
- Автоматизация UI Google/YouTube может противоречить общим условиям использования автоматизированного доступа.
- PoC допустим только для **собственного** experimental-канала, с ручным входом, без обхода CAPTCHA/блокировок.
- При `automation_blocked` / challenge — **стоп**, оставить ручной Download + импорт MP4 в UI.
- Cookies профиля не передаются сторонним сервисам; хранить с ограниченными правами ОС.

## Компоненты

- `browser-retriever/src/studio-cli.mjs` / `studio-retrieve.mjs` / `studio-poc-once.mjs`
- `internal/retrieval/studio.go` (`playwright_youtube_studio`)
- `cmd/studiopoc` — decode/decrypt/SHA локально
- Env: `STUDIO_RETRIEVER_JS`, `STUDIO_BROWSER_PROFILE`, `STUDIO_RETRIEVER_HEADLESS`

## Локальный сценарий

```bash
cd browser-retriever
npm ci
npx playwright install chromium
npm run studio-login
# в открывшемся окне: Google login + 2FA вручную

# one-shot (headed, устойчивее к session flake):
node src/studio-poc-once.mjs --video-id=KgUPTeDEf_o --out-dir=../tmp/studio-out

# или CLI:
node src/studio-cli.mjs --video-id=KgUPTeDEf_o --out-dir=../tmp/studio-out --profile-dir=./.studio-profile --headed
ffprobe -show_streams tmp/studio-out/*.mp4
```

Decode/decrypt: `go run ./cmd/studiopoc ...` (без смены codec/manifest).

## Результаты PoC (2026-10-20)

| Шаг | Результат |
|-----|-----------|
| Ручной login + 2FA в persistent profile | **OK** |
| Studio Download `KgUPTeDEf_o` | **OK** — MP4 15 804 908 байт |
| Разрешение Studio MP4 | **1280×720** H.264, ~60.4 с (официально 720p/360p) |
| Исходный encode | 1920×1080 codec v2 |
| Decode (первый проход) | FAIL — не пробовали сетку 240×135 на кадре 720p |
| Decode после фикса `candidateLayouts` | **OK** |
| SHA-256 `ec869758…e5bdaa5f` | **совпал** |

Подробности: [`codec-v3-studio-compatibility.md`](codec-v3-studio-compatibility.md).

### Причина первого FAIL

Не необратимая потеря битов, а пробел в кандидатах сетки. `Extract` проецирует логические cols×rows на фактические пиксели; не хватало кандидата v2 `1920×1080/b8`.

### VPS

Studio retriever + исправленный decode технически готовы к проверке на VPS (отдельный deploy). Нужен однократный headed login/2FA; temp в `/dev/shm`; лимит 5 download/видео/сутки.

**Deploy / push в этой задаче не выполнялись.**
