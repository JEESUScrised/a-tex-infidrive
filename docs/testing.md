# Тестирование

Отдельные бинарные образцы в репозитории не хранятся. Тесты создают файлы во временном каталоге: текст, байты `0x00…0xFF`, пустой файл и `crypto/rand`.

Запуск из корня модуля:

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

Тесты с FFmpeg вызывают `exec.LookPath("ffmpeg")`. Если утилиты нет в `PATH`, такой тест пишет `SKIP` и не падает. На Windows для прогона с реальным кодированием в `PATH` должны быть `ffmpeg` и `ffprobe` (сборка с libx264).

## Что проверяют тесты

| Требование | Где |
| --- | --- |
| Текст, имя и MIME | `internal/codec` `TestTextRoundTrip` |
| Бинарные данные `0x00…0xFF` | `TestBinaryRoundTrip` |
| Пустой файл | `TestEmptyFile` |
| `crypto/rand` и SHA-256 | `TestRandomSHA256` |
| gzip | `TestGzipRoundTrip` |
| Повреждённый пакет не выдаётся за успех | `TestCorruptPacketRejected` |
| Дубликаты кадров | `TestDuplicateFrames` |
| Повтор пакета | `TestPacketReplica` |
| Один потерянный пакет и XOR | `TestParityRecoversOne` |
| Два потерянных пакета в группе | `TestTwoMissingPacketsFail` |
| Чёрный кадр пропускается | `TestBlackFrameSkipped` |
| Сдвиг яркости и масштаб 640×360 | `internal/video` |
| Ошибка отсутствующего и битого FFmpeg | `internal/ffmpeg` |
| Полный MP4 round-trip | `TestFFmpegRoundTrip` |
| Повторное сжатие CRF 23, CRF 28 и 640×360 | `TestLossyReencode` |
| Очередь заданий и лимит | `internal/jobs` |
| HTTP: загрузка, 413, 404, encode | `internal/httpapi` |
| Имена файлов и path traversal | `internal/storage` |
| YouTube не притворяется настроенным | `internal/youtube` |

Кадры без FFmpeg собираются в памяти: `EncodeFrames` пишет RGB, тесты портят отдельные кадры, `DecodeFrames` читает их обратно.

## Прогон 20 октября 2026

Машина: Windows 10, Go 1.21, `CGO_ENABLED=0`. FFmpeg — переносимая сборка BtbN `ffmpeg-master-latest-win64-gpl` (libx264), добавленная в `PATH` только на время прогона. Каталог `.tools` в git не входит.

`go fmt ./...` и `go vet ./...` завершились без замечаний.

`go test -count=1 ./...` — все пакеты `ok`:

- `internal/codec` — в том числе `TestFFmpegRoundTrip` и `TestLossyReencode`
- `internal/ffmpeg`, `internal/httpapi`, `internal/jobs`, `internal/packets`, `internal/storage`, `internal/video`, `internal/youtube`

Отдельный подробный прогон:

- `TestFFmpegRoundTrip` — PASS, 3.21 с
- `TestLossyReencode/crf23` — PASS
- `TestLossyReencode/crf28` — PASS
- `TestLossyReencode/scale640-crf23` — PASS

Условия повторного сжатия: исходный MP4 собран настройками по умолчанию (1280×720, блок 16×16, CRF 14, preset `veryfast`, FPS 15, GOP 1, повтор кадра 2, повтор пакета 2, XOR на каждые 8 пакетов). Затем `libx264` перекодировал его с обычным GOP: CRF 23, CRF 28 и масштаб 640×360 при CRF 23. SHA-256 восстановленного файла совпал с исходными байтами.

Дополнительно вручную проверены команды:

```text
ytdrive encode --input hello.txt --output hello.mp4
ytdrive decode --input hello.mp4 --output restored.txt
ytdrive verify --original hello.txt --restored restored.txt
```

`verify` напечатал `OK` и один и тот же SHA-256. Собраны `bin/ytdrive.exe` и `bin/server.exe`.

Веб-сервер на `http://127.0.0.1:8091` показал «Сервер работает, FFmpeg найден» и текст «загрузка на YouTube не настроена». Через страницу запущено кодирование `note.txt`: задание дошло до 100%, появились ссылки `mp4` и «скачать mp4». Загрузка этого MP4 в `POST /api/recover` вернула те же 12 байт и тот же SHA-256. Отдельная проверка `report.txt` подтвердила заголовок `Content-Disposition: attachment; filename="report.txt"` у восстановленного файла.

`go test -race ./...` на этой машине не выполнился: Go 1.21 на Windows ответил `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`, а компилятор C (`gcc`) не установлен. Детектор гонок поэтому не запускался. Это ограничение среды, а не пропущенный тест пакета.

## Что повторное сжатие не доказывает

Успех CRF 23, CRF 28 и 640×360 получен на коротком ролике из плоских блоков 16×16. Он не доказывает, что любой ролик после обработки YouTube восстановится: длинный GOP, сильное размытие и кадры, где соседние блоки перепутаны, по-прежнему могут уничтожить бит. Декодер в таком случае пропускает кадр или отказывается публиковать файл, если SHA-256 не сошёлся.
