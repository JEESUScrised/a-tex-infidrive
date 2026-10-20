# Миграция: облачный диск поверх StorageProvider

## Цель

Пользовательский продукт — файловое облако. YouTube/FFmpeg/MP4 — внутренний experimental backend, не UI.

## Принципы

- На VPS постоянно: только SQLite и метаданные.
- Пользовательские байты: только внешний `StorageProvider` (S3/R2).
- Временные буферы: память / `TEMP_PATH` с автоочисткой, не `data/objects`.
- Бинарный формат YTDrive v1 и пакет `internal/codec` не меняем.
- YouTube OAuth/upload сохраняем как admin/experimental provider.

## Этапы

1. Интерфейс `blobstore.StorageProvider` + Memory + S3.
2. Схема: folders, drive_files, share_links, trash (аддитивные миграции).
3. API `/api/drive/*` и публичный `/s/{token}`.
4. SPA: Мои файлы / Общие / Корзина / Настройки; admin отдельно.
5. Старые `/api/files` encode/youtube — только admin/legacy.

## Риски

- Без S3 credentials в production сервер не принимает пользовательские файлы.
- YouTube provider не даёт разрешённый download → не заявляем как готовый storage backend.
