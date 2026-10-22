# NOTICE — происхождение и лицензии

## Собственный код

A-Tex InfiDrive — учебный проект (дисциплина «Разработка программного обеспечения на Go»).
Автор: **Nikita** / проект **A-Tex Group**.

Исходный код проекта распространяется на условиях **GNU General Public License v3.0** — см. файл `LICENSE`.

## Вдохновение / совместимость идеи

Идея хранения бинарных данных в видеокадрах (пакеты, CRC, один бит на блок яркости) вдохновлена проектом
[PulseBeat02/yt-media-storage](https://github.com/PulseBeat02/yt-media-storage) (GPL-3.0).

В A-Tex InfiDrive:

- раскладка байтов заголовка/пакетов, алгоритм кодирования/декодирования и веб-инфраструктура **написаны заново**;
- сохранена предметная совместимость идеи, а не дословное копирование исходников `yt-media-storage`;
- поэтому проект консервативно лицензирован под **GPL-3.0**, а не MIT.

Ссылка на исходный проект сохранена в `internal/format/format.go`.

## Зависимости

Лицензии зависимостей сохраняют силу своих проектов, в том числе:

| Компонент | Назначение | Лицензия (типично, см. upstream) |
|-----------|------------|-----------------------------------|
| Go stdlib + `golang.org/x/*`, `google.golang.org/api`, `oauth2` | Backend, YouTube Data API | BSD-3-Clause |
| `modernc.org/sqlite` | SQLite без CGO | BSD-3-Clause |
| React / React DOM / Vite / TypeScript | Frontend | MIT |
| `@gravity-ui/uikit`, `@gravity-ui/icons` | UI | MIT |
| `@tanstack/react-query`, `react-router-dom` | Frontend | MIT |
| Playwright | Browser worker (`browser-retriever`) | Apache-2.0 |
| FFmpeg / libx264 (внешний процесс, не линкуется) | Кодирование MP4 | См. сборку FFmpeg (LGPL/GPL в зависимости от опций) |

Точные тексты лицензий зависимостей находятся в их репозиториях и в `package-lock.json` / `go.sum`.
FFmpeg вызывается как внешний процесс и не входит в состав репозитория.
