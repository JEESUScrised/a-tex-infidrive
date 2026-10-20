# A-Tex InfiDrive

Продукт **A-Tex Group**: веб-сервис на Go, который кодирует произвольный файл в MP4 (H.264) и восстанавливает его обратно. Учебное ядро совместимо с бинарным форматом **YTDrive v1** (имя файла БД `ytdrive.db` сохранено).

Публичный сайт: `https://a-tex-infidrive.ru`  
Приложение: `https://app.a-tex-infidrive.ru`

Идея близка к [yt-media-storage](https://github.com/PulseBeat02/yt-media-storage) (GPL-3.0): данные режутся на пакеты, проверяются CRC и рисуются блоками кадра. Код написан заново. Формат: один бит на блок 16×16 по яркости, избыточность — повтор пакетов и XOR.

Загрузка на YouTube — только через официальный YouTube Data API v3 и OAuth 2.0. Неофициальное скачивание роликов не реализуется. Подробности: `docs/youtube-integration.md`. Production: `docs/deployment.md`.

## Зависимости

- Go 1.21 или новее
- FFmpeg с libx264 и желательно ffprobe
- Node.js 20+ (сборка frontend)
- Windows или Linux
- CGO не нужен для сборки и обычных тестов

## Безопасность (single-admin)

В production API закрыт сессией администратора (bcrypt, HttpOnly cookie, CSRF, rate limit).  
OAuth-токены YouTube шифруются at rest. Публичны маркетинговые страницы и lean `GET /api/health`.

См. `.env.example`, `.env.production.example`. Хеш пароля: `go run ./cmd/hashpw 'пароль'`.

## FFmpeg

1. `FFMPEG_PATH` / `FFPROBE_PATH` или `YTDRIVE_FFMPEG` / `YTDRIVE_FFPROBE`
2. локальный `.tools/*/bin`
3. системный `PATH`

## Frontend

```powershell
cd frontend
npm install
npm run build
cd ..
go run .\cmd\server
```

Dev (Vite proxy → `:8092`):

```powershell
go run .\cmd\server
cd frontend; npm run dev
```

## Запуск (локально)

По умолчанию сервер слушает `127.0.0.1:8092`.

```powershell
go run .\cmd\server
```

Откройте http://127.0.0.1:8092

## Production

См. `docs/deployment.md`, `deploy/deploy.sh`, `deploy/caddy/Caddyfile`, `deploy/systemd/infidrive.service`.

Каталог данных по умолчанию — `data/` (production: `/var/lib/infidrive`). SPA — `frontend/dist`. Публичный сайт — `site/`.

## HTTP API

Публично:

- `GET /api/health` — статус без чувствительных путей
- `GET /api/auth/csrf`, `GET /api/auth/me`, `POST /api/auth/login`

После входа администратора:

- файлы, задания, recover, YouTube, `GET /api/system`
- `GET /api/youtube/callback` — OAuth redirect (нужна сессия)

## Конфигурация

Скопируйте `.env.example` → `.env`. Секреты не коммитятся. Алиасы `YTDRIVE_*` сохранены.

## Проверки

```bash
go fmt ./...
go vet ./...
go test ./...
cd frontend && npm run typecheck && npm test && npm run build
```

На Windows `go test -race` нужен C-компилятор.

## Ограничения

- Экспериментальное хранение через YouTube не гарантирует сохранность или бессрочность.
- Без Google credentials загрузка на YouTube отключена.
- Юридические реквизиты для публичных страниц: `docs/legal-placeholders.md`.

Описание байтов — `docs/file-format.md`, архитектура — `docs/architecture.md`, деплой — `docs/deployment.md`.
