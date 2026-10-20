# Large Files / Storage Engine v3 — benchmark и итоги

Дата измерений: 2026-10-20 (локально, Windows + FFmpeg N-127696).  
Данные: `crypto/rand` (несжимаемые). Инструмент: `go run ./cmd/codecbench`.

## Что изменено

1. **Benchmark** (`cmd/codecbench`) — encode/decode/CRF/scale/block.
2. **Codec profile v2** (`codec.DefaultOptionsV2`): 1080p, block 8, CRF 23, FrameRepeat 1, PacketRepeat 2, format version 2. YTDrive v1 сохранён (`DefaultOptions`).
3. **Chunked storage**: SQLite `file_chunks` + потоковая загрузка частями (`pipeline.UploadChunkedStream`).
4. **Шифрование частей**: AES-256-GCM + AAD (`contentcrypto.SealAAD` / `ChunkAAD`).
5. **Лимиты**: `MAX_UPLOAD_MIB` / `MAX_CHUNK_MIB` / `MAX_TEMP_MIB`, ожидание YouTube processing с backoff (до 30 мин).
6. **Frontend**: подсказка и клиентский потолок 50 МиБ; прогресс при загрузке.
7. **Admin**: `GET /api/admin/files/{id}/manifest`.

YouTube mass upload и production deploy **не** выполнялись.

## Сравнение codec v1 и denser / v2

### v1 baseline (720p, block 16, FR=2, PR=2, ChunkSize≈364)

| Размер | CRF | Encode | Длительность MP4 | Размер MP4 | МБ/мин | SHA |
|--------|-----|--------|------------------|------------|--------|-----|
| 64 KiB | 16/23/28 | ~3 с | 81.6 с | ~3.2–3.5 МиБ | **0.046** | ok |
| 256 KiB | 16/23/28 | ~11–13 с | 324.8 с | ~12.8–14.1 МиБ | **0.046** | ok |
| 1 МиБ | 16/23/28 | ~42 с | **1297 с (~21.6 мин)** | ~51–57 МиБ | **0.046** | ok |

Полезная ёмкость кадра v1: **400 B/frame**.

### Denser (1080p, block 8, FR=1, PR=1) — исследование

| Размер | CRF | Encode | Длительность | MP4 | МБ/мин | SHA |
|--------|-----|--------|--------------|-----|--------|-----|
| 64 KiB | 23/28 | ~0.3–0.4 с | 2.1 с | ~0.55 МиБ | 1.79 | ok |
| 1 МиБ | 23/28 | ~3.6 с | 30.3 с | ~8.5–8.8 МиБ | **1.98** | ok |
| 2 МиБ | 23/28 | ~7 с | 60.3 с | ~17 МиБ | **1.99** | ok |
| 5 МиБ | 23 | 17.2 с | 150.6 с | 42.3 МиБ | **1.99** | ok |

Ёмкость кадра: **3960 B/frame**. Ускорение vs v1: **~43×**.

### Production profile v2 (PacketRepeat=2 для устойчивости encrypted payload)

После roundtrip с AES-GCM на 5 МиБ при PR=1 наблюдались потери пакетов на H.264.  
Итоговый профиль: **PR=2**, FR=1 → ожидаемая скорость **~1.0 МБ/мин** (всё ещё ≥0.5 МБ/мин).

Локальные тесты pipeline (Fake YouTube):

| Тест | Результат |
|------|-----------|
| 1 МиБ, chunk 512 KiB, multi-part | PASS (~11 с) |
| 5 МиБ, один chunk 5 МиБ | PASS (~52 с) |
| legacy tiny e2e + manual restore | PASS |

## Целевые показатели для 50 МиБ

| Метрика | Цель | Оценка по профилю v2 (PR=2) |
|---------|------|------------------------------|
| Полезная скорость | ≥0.5 МБ/мин | ~1.0 МБ/мин |
| Суммарная длительность видео | ≤100 мин | ~50 мин |
| Число роликов (chunk 5 МиБ) | желательно ≤10 | **10** |
| SHA-256 roundtrip | обязателен | подтверждён на 1 и 5 МиБ |

**50 МиБ полный локальный encode всех частей** в этой сессии не прогонялся end-to-end как один прогон (долго + квота диска tmp); экстраполяция с 5 МиБ × 10 и chunked unit-test.  
Рекомендуется перед VPS: `go test ./internal/pipeline/ -run 5MiB` и отдельный ручной прогон 10/25/50 МиБ с FakeProvider.

## Память / tmpfs

- Сервис: `MemoryMax=3G`.
- Один chunk 5 МиБ → MP4 порядка **40–90 МиБ** (зависит от PR/CRF) — укладывается в `/dev/shm` и `MAX_TEMP_MIB=1024`.
- Одновременно держится **одна** часть (encode → upload → cleanup).
- Полный файл 50 МиБ при download собирается в RAM-буфере (≤50 МиБ) — допустимо при MemoryMax 3G.

## Ограничения YouTube

- Каждая часть = отдельный public-ролик (experimental).
- Processing wait: polling + backoff, дедлайн по умолчанию **30 мин** (было 2 мин).
- Имена роликов: `infidrive-{fileID[:8]}-c{NN}` — без имени пользовательского файла.
- Авто-retrieval (Playwright/SSYouTube) для 1080p-источника может отдавать низкое разрешение: **не гарантирует** decode v2; остаётся manual Studio fallback.
- Массовые live-загрузки **не** запускались.

## Готовность к VPS

| Пункт | Статус |
|-------|--------|
| Код chunked + v2 | готов локально |
| Миграции SQLite | совместимы (ADD COLUMN / CREATE TABLE IF NOT EXISTS) |
| Env: `MAX_UPLOAD_MIB=50`, `MAX_CHUNK_MIB=5`, `EXPERIMENTAL_MAX_KB=51200` | нужно выставить на VPS при деплое |
| Production deploy | **не выполнен** (ждёт подтверждения) |
| Live YouTube 50 МиБ | **не выполнен** (ждёт подтверждения) |

## Коммиты (ожидаемые логические)

1. benchmark + docs measurements  
2. codec v2 profile + format versioning  
3. chunked pipeline + SQLite manifest + crypto AAD  
4. frontend 50 MiB UX + admin manifest  

## Оставшиеся проблемы

1. Полный автоматический тест **50 МиБ** ещё не прогнан как один CI-job.
2. SSYouTube/Playwright могут не сохранять 1080p — риск для auto-restore v2.
3. Resume недостающей части с браузера (повторная досылка байт) — метаданные missing indexes есть, UX resume неполный.
4. Прямой pipe FFmpeg→YouTube resumable без tmp-файла не реализован (используется один tmp MP4 на часть).
5. DCT / multi-bit / Reed-Solomon зарезервированы в `format`, но не реализованы в v2 (плотность достигнута геометрией + меньшей избыточностью).
