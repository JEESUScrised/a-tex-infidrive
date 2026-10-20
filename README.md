# YTDrive

Учебный веб-сервис на Go: произвольный файл превращается в MP4 с H.264, а из этого ролика файл собирается обратно.

Идея близка к [yt-media-storage](https://github.com/PulseBeat02/yt-media-storage) (GPL-3.0): данные режутся на пакеты, пакеты проверяются контрольной суммой и рисуются блоками кадра. Код YTDrive написан заново. Оригинал рассчитан на lossless FFV1 и fountain codes. Здесь свой формат **YTDrive v1**: один бит на блок 16×16, бит читается по средней яркости, избыточность — повтор пакетов и XOR.

Неофициальное скачивание с YouTube не реализовано и не планируется. В `internal/youtube` оставлен интерфейс будущей загрузки через официальный YouTube Data API.

## Зависимости

- Go 1.21 или новее
- FFmpeg с libx264 и желательно ffprobe
- Windows или Linux
- CGO не нужен для сборки и обычных тестов

## FFmpeg

Поиск утилит (без обхода всего диска):

1. флаги `--ffmpeg` / `--ffprobe` или переменные `FFMPEG_PATH` / `FFPROBE_PATH`
2. `YTDRIVE_FFMPEG` / `YTDRIVE_FFPROBE`
3. локальный каталог `.tools/*/bin` рядом с рабочей директорией проекта
4. системный `PATH`

Пример локальной раскладки (каталог `.tools` в git не входит):

```text
.tools/
  ffmpeg-…-gpl/
    bin/
      ffmpeg.exe   # Linux: ffmpeg
      ffprobe.exe  # Linux: ffprobe
```

При старте сервер пишет диагностику в лог и отдаёт её в `GET /api/health` (`ffmpeg`, `ffmpeg_path`, `ffmpeg_source`).

## Запуск на Windows

```powershell
cd путь\к\проекту
go run .\cmd\server
```

Откройте http://localhost:8080

Если FFmpeg лежит в `.tools`, достаточно запускать сервер из корня проекта. Иначе:

```powershell
$env:FFMPEG_PATH = "C:\path\to\ffmpeg.exe"
$env:FFPROBE_PATH = "C:\path\to\ffprobe.exe"
go run .\cmd\server
```

Сборка:

```powershell
go build -o bin\ytdrive.exe .\cmd\ytdrive
go build -o bin\server.exe .\cmd\server
```

## Запуск на Linux

```bash
cd /path/to/project
go run ./cmd/server
```

```bash
export FFMPEG_PATH=/usr/bin/ffmpeg
export FFPROBE_PATH=/usr/bin/ffprobe
go run ./cmd/server
```

```bash
go build -o bin/ytdrive ./cmd/ytdrive
go build -o bin/server ./cmd/server
```

Каталог данных по умолчанию — `data/`. Страница лежит в `web/`, пользовательские файлы — только в `data/`.

## Командная строка

```bash
ytdrive encode --input file.zip --output video.mp4
ytdrive decode --input video.mp4 --output restored.zip
ytdrive verify --original file.zip --restored restored.zip
```

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
| `--ffmpeg`, `--ffprobe` | авто | пути к утилитам |

## HTTP API

- `GET /api/health`
- `POST /api/files` — multipart-поле `file`
- `GET /api/files`
- `GET /api/files/{id}`
- `GET /api/files/{id}/download`
- `POST /api/files/{id}/encode`
- `GET /api/files/{id}/video`
- `GET /api/jobs`
- `GET /api/jobs/{id}`
- `POST /api/jobs/{id}/cancel`
- `GET /api/jobs/{id}/download`
- `POST /api/recover` — multipart-поле `file` с MP4
- `GET /api/youtube` — загрузка не настроена

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

Тесты сами находят FFmpeg через `Resolve` (в том числе `.tools`). Если утилит нет, FFmpeg-тесты пишут `SKIP`. На Windows `go test -race` нужен C-компилятор (CGO). Подробные результаты — в `docs/testing.md`.

## Ограничения

- В кадре 720p помещается 400 байт полезных данных. Большой файл даёт длинное видео.
- Устойчивость к повторному сжатию H.264 подтверждена тестами для CRF 23, CRF 28 и масштаба 640×360 на коротких роликах с настройками по умолчанию. Это не гарантия для любого ролика после YouTube.
- Загрузка на YouTube в этой версии не реализована.

Описание байтов — в `docs/file-format.md`, архитектура — в `docs/architecture.md`.
