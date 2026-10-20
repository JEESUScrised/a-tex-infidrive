# Тестирование

Отдельные бинарные образцы в репозитории не хранятся. Тесты создают файлы во временном каталоге.

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

FFmpeg ищется через `ffmpeg.Resolve`: флаги и переменные `FFMPEG_PATH` / `FFPROBE_PATH` / `YTDRIVE_*`, затем `.tools/*/bin`, затем `PATH`. Если утилит нет, FFmpeg-тесты пишут `SKIP`.

## Автоматические проверки

| Сценарий | Результат |
| --- | --- |
| Текст → MP4 → файл, SHA-256 | PASS |
| ZIP → MP4 → файл, SHA-256 | PASS |
| Бинарный шаблон → MP4 → файл | PASS |
| `crypto/rand` 8 КиБ → MP4 → файл | PASS |
| Пустой файл → MP4 → файл | PASS |
| Повторное сжатие CRF 23 | PASS |
| Повторное сжатие CRF 28 | PASS |
| Масштаб 640×360 + CRF 23 | PASS |
| Повреждённый кадр полезной нагрузки + XOR | PASS |
| Отсутствующий кадр + XOR | PASS |
| Отмена задания (`POST /api/jobs/{id}/cancel`) | PASS |
| Ошибка отсутствующего FFmpeg | PASS |
| Повторное открытие SQLite | PASS |
| YouTube не притворяется настроенным | PASS |

## Прогон 20 октября 2026

Машина: Windows 10, Go 1.21, `CGO_ENABLED=0`. FFmpeg из `.tools` (источник `local:.tools`), без добавления в системный PATH.

`go fmt ./...`, `go vet ./...` — без замечаний.

`go test -count=1 ./...` — все пакеты `ok`.

Интеграция (`TestIntegrationFullCycleSHA256`, `TestIntegrationResilienceTable`):

| Проверка | SHA-256 / исходный размер | Статус |
| --- | --- | --- |
| text | совпал | PASS |
| zip | совпал | PASS |
| binary | совпал | PASS |
| random | совпал | PASS |
| empty | совпал | PASS |
| reencode-crf23 | совпал | PASS |
| reencode-crf28 | совпал | PASS |
| scale-640x360-crf23 | совпал | PASS |
| damaged-payload-frame-with-parity | совпал | PASS |
| missing-frame-with-parity | совпал | PASS |

Веб/API на `:8092`:

- health: `ffmpeg: true`, `ffmpeg_source: local:.tools`
- encode `cycle.txt` → SHA-256 исходника и результата совпали
- recover → `Content-Disposition: filename="cycle.txt"`, тело `web full cycle`
- cancel длинного encode → статус `cancelled`, сообщение `отменено`
- UI: «Сервер работает, FFmpeg найден (local:.tools).», ссылки mp4 / скачать файл, YouTube «не настроена»

`go test -race ./...` на этой машине не выполнился: нужен CGO и компилятор C. Это ограничение среды.

## Что повторное сжатие не доказывает

Успех CRF 23/28 и 640×360 получен на коротких роликах с плоскими блоками 16×16 и избыточностью по умолчанию. Сильное размытие или длинный GOP на стороне YouTube по-прежнему могут уничтожить бит. Тогда кадр пропускается или файл не публикуется, если SHA-256 не сошёлся.
