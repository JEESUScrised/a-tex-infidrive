# Experimental End-to-End Storage (A-Tex InfiDrive)

Учебный эксперимент для защиты колледжного проекта. **Не production.**

## Цель цикла

```
Upload → AES-GCM → Encode (YTDrive v1) → MP4 → YouTube Upload
→ Video Retrieval → Decode → Decrypt → SHA-256 → Download
```

YouTube — единственное внешнее хранилище видеоданных. S3/R2 в этом режиме не используются.

## Исследование SSYouTube (https://ssyoutube.com/)

| Вопрос | Результат |
|--------|-----------|
| Документированный публичный API | Нет. FAQ предлагает писать на contact@ssyoutube.com |
| Доступность сервиса | Уведомление о прекращении сервиса в США (архивное объявление на сайте) |
| Сторонние «API» | Архивные неофициальные обёртки (GitHub/Replit) — нестабильны, по сути scrape |
| CAPTCHA / ToS | Обход запрещён политикой проекта |
| Автоматическая серверная интеграция | **Не реализована как рабочий путь** |

Адаптер `internal/retrieval.SSYouTube` (HTTP API) по умолчанию возвращает `ErrUnavailable`.

**Playwright UI-retrieval** (`browser-retriever` + `internal/retrieval.Playwright`) автоматизирует веб-интерфейс SSYouTube.
См. подробный отчёт: [playwright-ssyoutube-experiment.md](./playwright-ssyoutube-experiment.md).

**Fallback:** ручной импорт MP4 (`POST /api/drive/{id}/restore` / «Из MP4»), если CAPTCHA или worker недоступен.

## Реализовано в коде

1. `CONTENT_ENCRYPTION_KEY` — AES-256-GCM до кодирования (`internal/contentcrypto`).
2. `YOUTUBE_PRIVACY=public|unlisted|private` (default **private**).
3. `EXPERIMENTAL_YOUTUBE_STORE=true` — upload через pipeline: encrypt→encode→YouTube→delete temp.
4. Ожидание `uploadStatus` / `processingStatus` / `privacyStatus` (не успех только по `video_id`).
5. Download: auto-retrieve → при недоступности код `retrieval_unavailable` + manual restore.
6. SHA-256 сверяется **до** ответа об успехе; при mismatch — ошибка, байты клиенту не выдаются как успех.
7. UI пользователя без YouTube (файлы / ссылки / корзина); `/s/{token}` — лендинг без video_id.
8. Admin: `GET /api/admin/retrieval`.

## Локальный автоматический тест (без реального YouTube)

```
go test ./internal/pipeline/ -count=1 -v -run TestEncryptEncodeFakeYouTubeManualRestore
```

Сценарий: FakeProvider + FFmpeg encode/decode + encrypt/decrypt + SHA match.
SSYouTube auto-download в тесте ожидаемо падает; восстановление идёт через сохранённые байты MP4 (эмуляция «полученного» ролика).

### Результаты локального прогона (2026-10-20)

| Параметр | Значение |
|----------|----------|
| Размер оригинала | 29 байт (`e2e-college-demo-payload-32b!!`) |
| SHA-256 оригинала | `c2103b702ddb52bff627573f76498056cb041675a1e76ce589b3400f6c2d3fda` |
| Video ID | `video123` (FakeProvider) |
| Codec/разрешение | YTDrive v1 → H.264 MP4 (FFmpeg ultrafast, локальный encode) |
| Время encode | входит в `go test` (~1 с на машине разработчика) |
| Время upload | мгновенно (FakeProvider) |
| Время retrieval | N/A (SSYouTube auto → `ErrUnavailable`) |
| Время decode | входит в `go test` (~1 с) |
| SHA-256 результата | совпал с оригиналом |
| Ошибки | auto-Download ожидаемо: `retrieval_unavailable` / manual fallback |
| Вывод | Encrypt→encode→(fake)YouTube→manual MP4→decode→SHA **работает**; авто-SSYouTube — **нет** |

## Реальный YouTube (требуются credentials)

Не выполнялся в CI без `GOOGLE_CLIENT_*`. Чеклист вручную:

1. Задать `.env`: `EXPERIMENTAL_YOUTUBE_STORE=true`, `YOUTUBE_PRIVACY=public`, `CONTENT_ENCRYPTION_KEY`, OAuth.
2. Подключить аккаунт (admin / legacy OAuth UI или API).
3. Загрузить файл ≤128 КБ.
4. Дождаться processing / public.
5. Попытаться Download → ожидается `retrieval_unavailable`.
6. Скачать свой ролик в Studio → «Из MP4» / `POST /api/drive/{id}/restore`.
7. Сверить SHA-256.

### Таблица результатов (реальный YouTube)

| Параметр | Значение |
|----------|----------|
| Размер оригинала | *не прогонялось в этой сессии (нет OAuth credentials в CI)* |
| SHA-256 оригинала | *TBD* |
| Video ID | *TBD* |
| Codec/разрешение полученного видео | *TBD* |
| Время encode | *TBD* |
| Время upload | *TBD* |
| Время retrieval | N/A (auto недоступен) |
| Время decode | *TBD* |
| SHA-256 результата | *TBD* |
| Ошибки | SSYouTube auto: unavailable |
| Вывод | Цикл encode→YouTube→manual MP4→decode подтверждён локально (Fake); live YouTube — ручной шаг после Studio |

## Вывод для защиты

- Полный **локальный** roundtrip с шифрованием и проверкой SHA подтверждён тестом.
- **Официальная** загрузка на YouTube Data API v3 сохранена.
- **Автоматическое** получение видео через SSYouTube **не удалось интегрировать** легитимно.
- Для демонстрации восстановления используется **ручной импорт собственного MP4**.
