# YTDrive

Учебный веб-сервис на Go: произвольный файл превращается в MP4 с H.264, а из этого ролика файл собирается обратно.

Идея близка к [yt-media-storage](https://github.com/PulseBeat02/yt-media-storage) (GPL-3.0): данные режутся на пакеты, пакеты проверяются контрольной суммой и рисуются блоками кадра. Код YTDrive написан заново. Оригинал рассчитан на lossless FFV1 и fountain codes. Здесь свой формат **YTDrive v1**: один бит на блок 16×16, бит читается по средней яркости, избыточность — повтор пакетов и XOR. Так проще объяснить формат на защите и пережить обычный H.264.

Неофициальное скачивание с YouTube не реализовано и не планируется. В `internal/youtube` оставлен интерфейс будущей загрузки через официальный YouTube Data API.

## Что нужно

- Go 1.21 или новее
- FFmpeg с libx264 в `PATH` (`ffmpeg` и желательно `ffprobe`)
- Windows или Linux, CGO не нужен

## Запуск сервера

Из корня репозитория:

```bash
go run ./cmd/server
```

Откройте http://localhost:8080

Каталог данных по умолчанию — `data/`. Его не нужно отдавать веб-сервером: страница лежит в `web/`, файлы пользователей — только в `data/`.

Сборка:

```bash
go build -o bin/ytdrive ./cmd/ytdrive
go build -o bin/server ./cmd/server
```

На Windows появятся `ytdrive.exe` и `server.exe`.

## Командная строка

```bash
ytdrive encode --input file.zip --output video.mp4
ytdrive decode --input video.mp4 --output restored.zip
ytdrive verify --original file.zip --restored restored.zip
```

Полезные флаги `encode`:

| Флаг | По умолчанию | Смысл |
| --- | --- | --- |
| `--fps` | 10 | частота кадров |
| `--width`, `--height` | 1280×720 | размер кадра |
| `--block` | 16 | сторона яркостного блока |
| `--crf` | 16 | качество H.264 |
| `--preset` | veryfast | preset libx264 |
| `--bitrate` | 0 | если задан, используется вместо CRF |
| `--gop` | 1 | каждый кадр ключевой |
| `--frame-repeat` | 2 | сколько раз записать один и тот же кадр |
| `--packet-repeat` | 2 | копии каждого пакета |
| `--parity-group` | 8 | XOR-группа, `0` выключает чётность |
| `--chunk` | 360 | байт данных в пакете |
| `--gzip` | выкл. | сжать содержимое перед укладкой |
| `--ffmpeg`, `--ffprobe` | из PATH | пути к утилитам |

`decode` сам находит сетку 1280×720. Если ролик кодировали другим разрешением, можно передать `--width`, `--height` и `--block`.

## HTTP API

- `GET /api/health`
- `POST /api/files` — multipart-поле `file`
- `GET /api/files`
- `GET /api/files/{id}`
- `GET /api/files/{id}/download` — исходный файл
- `POST /api/files/{id}/encode`
- `GET /api/files/{id}/video` — готовый MP4
- `GET /api/jobs`
- `GET /api/jobs/{id}`
- `GET /api/jobs/{id}/download` — результат задания
- `POST /api/recover` — multipart-поле `file` с MP4
- `GET /api/youtube` — всегда сообщает, что загрузка не настроена

Число одновременных процессов FFmpeg ограничено `YTDRIVE_WORKERS` (по умолчанию 1). Очередь — `YTDRIVE_QUEUE`.

## Конфигурация

Скопируйте `.env.example` в `.env`. Файл `.env` не коммитится. Секретов в проекте нет.

## Проверки

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

Тесты, которым нужен FFmpeg, пропускаются, если утилиты нет в `PATH`. Подробности и ограничения формата — в `docs/testing.md`.

## Ограничение объёма

В одном кадре 720p помещается 400 байт. Большой файл даёт длинное видео. Для демонстрации достаточно текста, небольшого архива или нескольких килобайт. Описание байтов — в `docs/file-format.md`, устройство программы — в `docs/architecture.md`.
