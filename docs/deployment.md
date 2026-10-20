# Развёртывание A-Tex InfiDrive

## Архитектура production

- Бинарь Go слушает только `127.0.0.1:8092`
- Caddy (или существующий reverse proxy) терминирует HTTPS на `80/443`
- `a-tex-infidrive.ru` — статический сайт из `site/`
- `app.a-tex-infidrive.ru` — SPA + REST API
- Данные: `/var/lib/infidrive` (SQLite `ytdrive.db`, objects, jobs, tmp)
- Пользователь Linux: `infidrive`
- Unit: `deploy/systemd/infidrive.service`

## Перед деплоем

1. Проверьте ОС, RAM, диск, занятость 80/443/8092, существующий Nginx/Caddy.
2. Настройте DNS (только при наличии IPv4; AAAA не добавляйте без рабочего IPv6):

| Имя | Тип | Значение |
| --- | --- | --- |
| `a-tex-infidrive.ru` | A | `<IPv4 VPS>` |
| `app.a-tex-infidrive.ru` | A | `<IPv4 VPS>` |

3. Дождитесь резолва (`dig +short a-tex-infidrive.ru`).
4. Заполните `/etc/infidrive/env` из `.env.production.example`.
5. Сгенерируйте секреты:

```bash
go run ./cmd/hashpw 'сильный-пароль'
openssl rand -base64 48   # SESSION_SECRET
openssl rand -hex 32      # TOKEN_ENCRYPTION_KEY
```

6. В Google Cloud Console добавьте redirect:
   `https://app.a-tex-infidrive.ru/api/youtube/callback`
   Dev: `http://localhost:8092/api/youtube/callback`

OAuth приложение можно оставить в Testing до отдельного решения о публикации.

## Сборка артефакта

```bash
cd frontend && npm ci && npm run build && cd ..
CGO_ENABLED=0 go build -o bin/infidrive ./cmd/server
```

## Деплой на сервере

```bash
export SRC_DIR=/path/to/release
sudo bash deploy/deploy.sh
sudo systemctl status infidrive
curl -fsS http://127.0.0.1:8092/api/health
curl -fsSI https://app.a-tex-infidrive.ru/
```

## Откат

```bash
sudo bash deploy/rollback.sh <release_id>
```

## Резервное копирование

`deploy.sh` делает snapshot SQLite в `/var/lib/infidrive/backups/<release_id>/`.
Дополнительно рекомендуется периодический `sqlite3 … .backup` и копирование `objects/`.

## Безопасность

- Публичны: маркетинговые страницы и lean `GET /api/health`
- Остальной API — только после входа администратора (session + CSRF)
- OAuth YouTube — только для авторизованного admin
- Токены Google шифруются `TOKEN_ENCRYPTION_KEY`
- Production не стартует со слабыми/пустыми секретами

## Юридические данные к заполнению

См. `docs/legal-placeholders.md`.
