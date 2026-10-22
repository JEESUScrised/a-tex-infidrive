# Codec v3 — совместимость с YouTube Studio (720p)

Дата: 2026-10-20. Deploy/push не выполнялись. Видео `KgUPTeDEf_o` и SQLite-манифесты не удалялись.

## 1. Удалось ли восстановить старый v2 из Studio 720p MP4?

**Да — полностью.**

| Шаг | Результат |
|-----|-----------|
| Источник | `tmp/studio-out/infidrive-a38ce031-c00.mp4` (Studio Download, ~15.8 MiB) |
| Сигнал YTDrive | найден (layout `1920×1080/b8` → cols=240×rows=135, проекция на 1280×720) |
| Decode | OK, payload 1 048 610 байт (ciphertext) |
| Decrypt AES-GCM + AAD | OK |
| SHA-256 plaintext | **`ec8697581526431ae2f6975dc790845d07ca43b61e9aaf76de067059e5bdaa5f`** — **совпал** |

Файл: `live-v3-1m.bin` / file_id `a38ce0310ffa406ea91037ab28d0dde5`.

## 2. Точная причина потери данных (раньше)

Это **не** была необратимая гибель битов после YouTube.

Декодер перебирал только сетки, кратные **фактическому** размеру кадра (1280×720 → 160×90 при block 8).  
Encode v2 пишет в заголовок **cols=240, rows=135**. `Extract` умеет проецировать логическую сетку на любой пиксельный размер через `blockRect`, но кандидат `1920×1080/b8` **не добавлялся**.

Итог: ErrNoSignal («кадры YTDrive не найдены»), хотя контраст luma ещё был.

Исправление: `candidateLayouts` всегда пробует известные профили v1/v2/v3 + сдвиги ±2 px в диагностике.

## 3. Алгоритмы масштабирования (этап 1)

Все варианты гонялись через FFmpeg `scale` → raw RGB24 **без** промежуточного lossy MP4:

| Режим | Markers / Header CRC OK | Decode + SHA |
|-------|-------------------------|--------------|
| native 1280×720 | OK (с проекцией 240×135) | **OK** |
| upscale neighbor → 1080p | OK | OK |
| bilinear → 1080p | OK | OK |
| bicubic → 1080p | OK | OK |
| lanczos → 1080p | OK | OK |

Для восстановления **достаточно native 720p + проекция**. Upscale не обязателен.

Диагностика: `go run ./cmd/codecdiag -in tmp/studio-out/….mp4`.

## 4. Параметры codec v3

Нативный профиль под официальный Studio Download (720p / 360p).

| Параметр | Значение |
|----------|----------|
| FormatVersion | **3** |
| Разрешение | **1280×720** |
| Block | **8** (выбран после сравнения 8/10/16) |
| FPS / GOP | 10 / 1 |
| CRF (encode) | 18 |
| FrameRepeat | 2 |
| PacketRepeat | 2 |
| ParityGroup | 8 (XOR FEC) |
| ChunkSize | 1704 (= PayloadCap 1740 − packet overhead 36) |
| Luma0 / Luma1 | 16 / 235 |
| Pixel mode | PixelLuma1 (как v1/v2) |
| Крипто | AES-256-GCM без изменений |

**Block 12:** `1280` и `720` не делятся на 12 — не использовался без смены разрешения.

Плотность (полезные данные / длительность MP4 при FR=2):

| Block | PayloadCap | ≈ MiB/мин (1 MiB тест) | Устойчивость CRF23/28/360p |
|-------|------------|-------------------------|----------------------------|
| **8** | 1740 | **~0.22** | все SHA OK |
| 10 | 1088 | ~0.13 | все SHA OK |
| 16 | 400 | ~0.05 | все SHA OK |

Выбран **block 8**: лучшая плотность среди Studio-safe вариантов при полной устойчивости в локальных стресс-тестах. Не максимум любой ценой (v2 ~1 MiB/мин на 1080p плотнее, но зависит от проекции после Studio).

Синхронизация: magic `YT1\x01` + CRC32 заголовка + калибровочная строка (адаптивный порог) + чтение **центра** блока (внутренняя ½) + FEC/повторы. Нет жёсткой привязки к абсолютным пиксельным координатам encode — сетка проецируется на фактический кадр.

## 5. Результаты CRF 23 / 28

Локальный стенд (`cmd/codecv3bench`, crypto/rand): encode v3 → re-encode libx264 CRF 23/28 → decode → SHA.

Для размеров **64 KiB, 256 KiB, 1 MiB, 5 MiB** и блоков **8/10/16**:

- `reenc_crf23` — **SHA OK**, corrupt_packets=0  
- `reenc_crf28` — **SHA OK**, corrupt_packets=0  
- `reenc_crf23_360p` / `reenc_crf28_360p` — **SHA OK** (синтетический даунскейл 640×360)

CSV: `tmp/codecv3bench/results.csv`.

## 6. Скорость кодирования (MiB/мин полезных данных)

Оценка по длительности выходного MP4 (logical frames × FrameRepeat / FPS):

| Размер | Block 8 | Block 10 | Block 16 |
|--------|---------|----------|----------|
| 1 MiB | **~0.22 MiB/мин** | ~0.13 | ~0.05 |
| 5 MiB | ~0.22 | ~0.13 | ~0.05 |

Encode wall-time (1 MiB / b8 / CRF18 / veryfast): порядка десятков секунд на dev-машине (см. `encode_sec` в CSV).

## 7. Длительность видео на 1 MiБ

| Профиль | Длительность | Размер MP4 (base CRF18) |
|---------|--------------|-------------------------|
| v3 720p/b8 FR=2 | **~277 с (~4.6 мин)** | ~35.6 МиБ |
| v3 720p/b10 | ~449 с | ~113 МиБ |
| v3 720p/b16 | ~1297 с | ~56 МиБ |

## 8. Локальные SHA-256

- Все комбинации bench 64k–5m × b8/10/16 × CRF23/28 × (±360p): **pass**  
- Подготовленный файл `tmp/live-v3-prep/live-v3-studio-1m.bin`:  
  SHA `6805f0a122ad020e39863e469c2e68e81ac47c1e5b37125c5d0a5bcb21adec75`  
  Encode v3 → CRF23 reenc → decode: **sha_ok=true**  
- Live Studio v2 (`KgUPTeDEf_o`): **sha_ok=true** (см. §1)

## 9. Совместимость со старыми файлами

| Версия | Encode | Decode |
|--------|--------|--------|
| v1 (720p/b16) | без изменений | да |
| v2 (1080p/b8) | без изменений | да (+ проекция на Studio 720p) |
| v3 (720p/b8) | новый default для experimental YouTube store | да |

`format.SupportedFrameVersion` = 1|2|3. Манифест `file_chunks.codec_version` / `drive_files.codec_version` пишут фактическую версию.

Криптосхема AES-GCM прежняя. Manifest v3 chunked pipeline сохранён.

Статусы восстановления (разделение):

| Поле | Смысл |
|------|--------|
| `status=ready` | загружен на YouTube, **доступен** для попытки restore (не значит «проверено») |
| `recovery_status=pending` | ещё не готов / в процессе upload |
| `recovery_status=available` | upload завершён, restore не верифицирован |
| `recovery_status=verified` | decode+decrypt+SHA OK |
| `recovery_status=failed` | ошибка восстановления |

Обычный UI по-прежнему показывает **файлы**, не ролики.

## 10. Что нужно для следующего настоящего теста YouTube

Подготовлено локально (без upload):

- `tmp/live-v3-prep/live-v3-studio-1m.bin` (1 МиБ crypto/rand)  
- `tmp/live-v3-prep/live-v3-studio-1m.mp4` (encode v3)  
- контрольный SHA выше  
