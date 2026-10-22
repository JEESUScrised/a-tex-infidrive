# A-Tex InfiDrive

Учебное приложение — экспериментальное файловое хранилище, написанное на Go.

Пользователь загружает файл через веб-интерфейс. Сервер шифрует данные (AES-256-GCM), преобразует их в видеопоток MP4 (H.264) и при включённом экспериментальном режиме использует YouTube как backend хранения. Восстановление идёт обратным путём: получение MP4, декодирование кадров, проверка целостности и расшифровка.

Проект создан в учебных целях и не является надёжным способом долговременного хранения файлов.

## Возможности

Реализовано:

- загрузка файлов через веб-интерфейс (файловый менеджер: папки, список, корзина);
- шифрование содержимого перед кодированием (AES-256-GCM, ключ задаётся в конфигурации);
- преобразование бинарных данных в MP4 и обратно (собственный кодек поверх FFmpeg/H.264);
- версии формата кадра: v1 (1280×720, блок 16), v2 (1920×1080, блок 8), v3 (1280×720, блок 8 — под скачивание из YouTube Studio);
- подключение канала YouTube по OAuth 2.0 и загрузка через официальный YouTube Data API v3;
- хранение метаданных и манифестов в SQLite (файл `data/ytdrive.db` создаётся локально и в репозиторий не входит);
- chunked storage: большие файлы режутся на части (по умолчанию до 5 МиБ), каждая часть — отдельный ролик, со своим SHA-256 и версией кодека;
- контроль целостности SHA-256 на уровне файла и отдельных частей;
- ссылки общего доступа (share links) с публичной страницей скачивания;
- статусы восстановления: `pending` → `available` → `verified` / `failed` (`ready` означает «загружен и доступен для попытки восстановления», а не «восстановление проверено»);
- экспериментальное восстановление: ручной импорт MP4, а также Playwright-worker для получения роликов владельца канала через YouTube Studio;
- веб-интерфейс на React + Gravity UI, вход администратора (сессия, bcrypt, CSRF).

## Технологический стек

- **Backend:** Go 1.21, `net/http`, SQLite (modernc.org/sqlite, без CGO)
- **Frontend:** React 19, TypeScript, Gravity UI, Vite, React Query
- **Видео:** FFmpeg (libx264) как внешний процесс
- **Браузерный worker:** Node.js + Playwright (Chromium)
- **Внешние API:** YouTube Data API v3 (OAuth 2.0)
- **Опционально:** S3-совместимое хранилище для неэкспериментального режима

## Архитектура

Загрузка:

```text
Browser → Go API → AES-GCM Encryption → Video Encoder (FFmpeg) → YouTube Data API
                                              │
                                              └→ SQLite (метаданные, chunk manifest)
```

Восстановление:

```text
YouTube Video → Retrieval (Studio / ручной MP4) → FFmpeg → Decoder → SHA-256 → Decrypt → Original File
```

Кодирование: файл режется на пакеты с CRC32, пакеты рисуются блоками яркости в кадрах MP4 (1 бит на блок), добавляются повторы пакетов/кадров и XOR-пакет чётности. Декодер читает среднюю яркость центральной части блока, порог берётся из калибровочных блоков кадра, геометрия сетки может проецироваться на фактическое разрешение видео (это нужно для роликов, скачанных из Studio в 720p).

## Структура проекта

```text
cmd/
  server/       HTTP-сервер (основная точка входа)
  ytdrive/      CLI: encode / decode / verify одного файла
  hashpw/       генерация bcrypt-хеша пароля администратора
  codecbench/   benchmark кодека на crypto/rand данных
  codecdiag/    диагностика MP4 (поиск сигнала YTDrive, upscale-эксперименты)
  codecv3bench/ сравнение блоков/CRF для codec v3
  studiopoc/    локальный PoC: decode Studio MP4 + decrypt + SHA-256
internal/
  codec/        encode/decode, планирование пакетов, сборка файла
  video/        раскладка кадра, заголовок, калибровка
  packets/      пакеты данных/метаданных/чётности, CRC32
  format/       версии формата и FEC-константы
  ffmpeg/       запуск FFmpeg/ffprobe, rawvideo pipe
  contentcrypto/ AES-256-GCM содержимого (с AAD по частям)
  cryptoutil/   шифрование секретов at rest
  pipeline/     chunked upload/restore, очередь заданий восстановления
  drive/        метаданные файлов, папки, share-ссылки, манифест частей
  storage/      SQLite-хранилище (в т.ч. OAuth-токены)
  youtube/      OAuth 2.0 + YouTube Data API v3, fake-провайдер для тестов
  retrieval/    получение MP4: manual, Playwright (SSYouTube legacy, Studio)
  blobstore/    провайдеры байтов: memory / S3 / youtube experimental
  jobs/         фоновые задания
  httpapi/      REST API, auth, статика SPA
  auth/         сессии, CSRF, rate limit
  config/       конфигурация из env/.env
frontend/       React + TypeScript + Gravity UI (Vite)
browser-retriever/ Playwright worker (Node.js, ESM)
site/           статические публичные страницы
deploy/         Caddy, systemd, скрипты развёртывания (для собственного сервера)
docs/           формат файла, архитектура, тесты, эксперименты
tests/          примечание о тестах (образцы генерируются на лету)
```

## Установка и запуск

### Требования

- Go **1.21** или новее (проверено на Go 1.21.5)
- Node.js **20+** и npm (проверено на Node 25 / npm 11)
- FFmpeg с libx264 и ffprobe в `PATH` (или задайте `FFMPEG_PATH` / `FFPROBE_PATH`)
- Для Playwright-worker: Chromium, устанавливаемый командой Playwright

Установка FFmpeg:

- Windows: скачать сборку с ffmpeg.org и добавить `bin` в `PATH`, либо положить в `.tools/` (проект умеет находить локальный FFmpeg);
- Debian/Ubuntu: `sudo apt install ffmpeg`;
- macOS: `brew install ffmpeg`.

### Конфигурация

```bash
cp .env.example .env
```

На Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Для локального запуска без YouTube правок `.env` не требуется: сервер стартует в обычном режиме, интерфейс и файловый менеджер работают, а загрузка на YouTube отключена, пока не заданы Google credentials и не включён экспериментальный режим.

Минимальные секреты для защищённого локального запуска при желании задаются в `.env` (`ADMIN_PASSWORD`, `SESSION_SECRET`, `TOKEN_ENCRYPTION_KEY`). Секреты в репозиторий не коммитятся.

### Backend

```bash
go run ./cmd/server
```

Сервер слушает `127.0.0.1:8092` (настраивается `SERVER_HOST` / `SERVER_PORT`), данные пишет в `data/`. Откройте http://127.0.0.1:8092 — если собран frontend, сервер отдаёт SPA из `frontend/dist`.

Сборка бинаря:

```bash
go build -o bin/infidrive ./cmd/server
```

### Frontend

```bash
cd frontend
npm install
npm run build
cd ..
```

Для разработки (Vite на :5173, proxy `/api` → :8092):

```bash
cd frontend
npm run dev
```

### Playwright worker (опционально)

Нужен только для экспериментального автоматического восстановления через браузер.

```bash
cd browser-retriever
npm ci
npx playwright install chromium
```

Первый вход в YouTube Studio выполняется вручную в открывшемся окне (включая 2FA):

```bash
npm run studio-login
```

Пароль программно не вводится, CAPTCHA и блокировки Google не обходятся. Профиль браузера (`.studio-profile/`) содержит сессию и **не должен попадать в Git** — он уже в `.gitignore`.

### Запуск без настоящего YouTube (offline/mock)

- Обычный режим: не задавайте `EXPERIMENTAL_YOUTUBE_STORE=true`. Файловый менеджер работает на локальном провайдере (`STORAGE_PROVIDER=memory` — только для разработки/тестов).
- В Go-тестах используется fake-провайдер YouTube (`internal/youtube/fake.go`), настоящий канал не нужен.
- CLI без сервера и без YouTube:

```bash
go run ./cmd/ytdrive encode --input file.bin --output video.mp4
go run ./cmd/ytdrive decode --input video.mp4 --output restored.bin
go run ./cmd/ytdrive verify --original file.bin --restored restored.bin
```

## Тестирование

Go (все пакеты, включая roundtrip с FFmpeg — нужен установленный FFmpeg):

```bash
go vet ./...
go test ./...
```

Что покрыто тестами (offline, без credentials):

- encode → decode roundtrip и совпадение SHA-256;
- устойчивость к повторному сжатию H.264 (CRF 23/28) и к даунскейлу 720p→360p для codec v3;
- восстановление v2-ролика после «скачивания из Studio» (1080p → 720p, проекция сетки);
- chunked storage: разрезание, manifest, AEAD по частям;
- пакеты/CRC, заголовки кадров, SQLite-миграции, HTTP API, OAuth (на fake/mocks).

Frontend:

```bash
cd frontend
npm run typecheck
npm test
npm run build
```

Live-тесты с настоящим YouTube в CI не выполняются: для них нужны собственные Google credentials, подключённый канал и ручное подтверждение. Результаты ручных live-проверок зафиксированы в `docs/` (например, `docs/codec-v3-studio-compatibility.md`).

### Подтверждённые результаты на момент публикации

- `go test ./...` — успешно (все пакеты).
- `go vet ./...` — успешно.
- Frontend: typecheck, unit-тесты (5), production build — успешно.
- Реальный цикл YouTube → Studio MP4 → decode → SHA-256: подтверждён на ролике 1 МиБ (codec v2), SHA-256 совпал. Для codec v3 тот же цикл подготовлен, но отдельный live-запуск ещё не выполнялся.

## Ограничения

- Проект экспериментальный и учебный. YouTube — не архивное хранилище: ролики могут удаляться, аккаунт — блокироваться, условия платформы — меняться.
- Предел загрузки 50 МиБ — настроенный лимит (файл режется на части). Полный live roundtrip на 50 МиБ **не подтверждён**; live-проверен файл 1 МиБ.
- YouTube перекодирует видео (H.264, изменение разрешения). Скачивание из Studio даёт 720p или 360p, а не исходный файл, поэтому кодек должен переживать перекодирование и смену разрешения.
- Автоматическое восстановление может не работать: Studio требует ручной первый вход и 2FA, возможны проверки безопасности и лимиты платформы (в т.ч. на число скачиваний). Ручной импорт MP4 остаётся рабочим запасным путём.
- Сторонние механизмы получения видео (не официальный Studio) имеют ограничения платформы и не являются основным способом восстановления; основной путь — скачивание собственных роликов владельцем канала.
- Режим `STORAGE_PROVIDER=memory` не сохраняет байты между перезапусками и предназначен только для разработки.
- Производительность кодека низкая (порядка 0,2 МиБ/мин у codec v3): это плата за устойчивость к перекодированию, а не способ быстрого хранения.
