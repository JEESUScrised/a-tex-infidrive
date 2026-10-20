# Интеграция с YouTube Data API v3

YTDrive загружает локально собранный MP4 через **официальный** YouTube Data API v3.
Неофициальное скачивание роликов и обход ограничений не реализуются и не планируются.

Официальный API **не отдаёт** видеопоток для произвольного скачивания. Восстановление файла после публикации идёт так:

1. Кодирование файла в MP4 в YTDrive.
2. Ручной запуск «Загрузить на YouTube» (privacy по умолчанию `private`).
3. Ожидание обработки на стороне YouTube.
4. Скачивание своего ролика в [YouTube Studio](https://studio.youtube.com/).
5. Импорт MP4 в YTDrive через «Восстановить из MP4».
6. Проверка SHA-256.

Автотест этого пути: `TestStudioDownloadRecoveryScenario` (повторное сжатие H.264 имитирует файл из Studio).

## Google Cloud

1. Создайте проект в [Google Cloud Console](https://console.cloud.google.com/).
2. Включите **YouTube Data API v3**.
3. Настройте экран согласия OAuth (тип External для личного аккаунта; добавьте свой email в тестовые пользователи).
4. Создайте учётные данные **OAuth client ID** → тип **Веб-приложение**.
5. Authorized redirect URIs:
   - локально: `http://localhost:8080/api/youtube/callback`
   - при другом порте поменяйте и `GOOGLE_REDIRECT_URL`.
6. Скопируйте Client ID и Client Secret в `.env` (файл не коммитится):

```env
GOOGLE_CLIENT_ID=....apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=http://localhost:8080/api/youtube/callback
```

Минимальные scope: `youtube.upload` и `youtube.readonly` (название канала).

## Запуск

```bash
go run ./cmd/server
```

В логе должно быть: `YouTube Data API: OAuth настроен`.
На странице — кнопка «Подключить Google». После согласия отображается название канала.

Загрузка **не** стартует сама: сначала «В видео», затем «Загрузить на YouTube».

## API

| Метод | Путь | Назначение |
| --- | --- | --- |
| GET | `/api/youtube` | статус OAuth и канал |
| GET | `/api/youtube/auth/start` | `{ "auth_url": "..." }` |
| GET | `/api/youtube/callback` | OAuth redirect |
| POST | `/api/youtube/disconnect` | удалить токены |
| POST | `/api/files/{id}/youtube` | поставить загрузку в очередь |
| GET | `/api/files/{id}/youtube` | статусы video_id / processing |

Токены хранятся в SQLite (`oauth_tokens`) внутри `YTDRIVE_DATA_DIR`. Не публикуйте каталог `data/`. Секреты и токены в логи не пишутся.

## Ограничения API

- Квоты YouTube Data API (upload дорого стоит по units).
- Resumable upload; повторный `videos.insert` после неопределённого ответа не делается наугад: `video_id` сохраняется сразу после успешного ответа.
- После перезапуска сервера задания в `running` / `uploading` / `processing` помечаются как failed («прервано перезапуском»).
- Реальная загрузка на ваш аккаунт требует ваших OAuth credentials. Без них провайдер остаётся `Unconfigured`, а автотесты используют mock / FakeProvider.

## Результаты тестов (локально)

Проверено mock-сервером и FakeProvider:

- OAuth state (CSRF, запрет повторного использования)
- сохранение / обновление / удаление токенов
- 401 → ErrAuth, 403 quota → ErrQuota, 503 → ErrTemporary
- фоновая загрузка, progress, `video_id`, privacy=`private`
- запрет повторной публикации
- миграция SQLite с полями `youtube_*`
- сценарий Studio recovery (lossy reencode + decode)

**Реальный upload на YouTube в этой среде не выполнялся** (нет пользовательских OAuth credentials в репозитории). После настройки `.env` проверьте вручную: подключение канала → загрузка короткого MP4 → появление `youtube_url` со статусом `private`.
