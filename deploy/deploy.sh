#!/usr/bin/env bash
# Развёртывание A-Tex InfiDrive на Linux VPS.
# Запускать от root на целевом сервере из распакованного релиза
# или передать RELEASE_DIR / REMOTE через обёртку.
set -euo pipefail

APP_NAME=infidrive
OPT_ROOT=/opt/infidrive
VAR_ROOT=/var/lib/infidrive
SITE_ROOT=/var/www/infidrive-site
ENV_FILE=/etc/infidrive/env
SERVICE=infidrive
RELEASE_ID="${RELEASE_ID:-$(date +%Y%m%d%H%M%S)}"
SRC_DIR="${SRC_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

echo "==> Проверка окружения"
id -u infidrive >/dev/null 2>&1 || useradd --system --home "$VAR_ROOT" --shell /usr/sbin/nologin infidrive
install -d -o infidrive -g infidrive -m 0750 "$VAR_ROOT" "$VAR_ROOT/tmp" /var/log/infidrive
install -d -m 0755 "$OPT_ROOT/releases" "$SITE_ROOT" /etc/infidrive

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Создайте $ENV_FILE из .env.production.example перед деплоем" >&2
  exit 1
fi

echo "==> Сборка (ожидается готовый бинарь или локальная сборка)"
RELEASE_DIR="$OPT_ROOT/releases/$RELEASE_ID"
install -d "$RELEASE_DIR/bin" "$RELEASE_DIR/frontend"

if [[ -x "$SRC_DIR/bin/infidrive" ]]; then
  install -m 0755 "$SRC_DIR/bin/infidrive" "$RELEASE_DIR/bin/infidrive"
elif [[ -f "$SRC_DIR/cmd/server/main.go" ]]; then
  (cd "$SRC_DIR" && CGO_ENABLED=0 go build -o "$RELEASE_DIR/bin/infidrive" ./cmd/server)
else
  echo "Не найден исходник или bin/infidrive" >&2
  exit 1
fi

if [[ -d "$SRC_DIR/frontend/dist" ]]; then
  cp -a "$SRC_DIR/frontend/dist/." "$RELEASE_DIR/frontend/dist/"
else
  echo "Нужен frontend/dist (npm run build)" >&2
  exit 1
fi

if [[ -d "$SRC_DIR/site" ]]; then
  rsync -a --delete "$SRC_DIR/site/" "$SITE_ROOT/"
fi

echo "==> Бэкап данных перед переключением"
BACKUP_DIR="$VAR_ROOT/backups/$RELEASE_ID"
install -d -o infidrive -g infidrive "$BACKUP_DIR"
if [[ -f "$VAR_ROOT/ytdrive.db" ]]; then
  sqlite3 "$VAR_ROOT/ytdrive.db" ".backup '$BACKUP_DIR/ytdrive.db'" 2>/dev/null \
    || cp -a "$VAR_ROOT/ytdrive.db" "$BACKUP_DIR/ytdrive.db"
fi

echo "==> Переключение current -> $RELEASE_ID"
ln -sfn "$RELEASE_DIR" "$OPT_ROOT/current"
chown -R infidrive:infidrive "$RELEASE_DIR"
install -m 0644 "$SRC_DIR/deploy/systemd/infidrive.service" /etc/systemd/system/infidrive.service
systemctl daemon-reload
systemctl enable "$SERVICE"
systemctl restart "$SERVICE"
systemctl --no-pager --full status "$SERVICE" || true

echo "==> Готово: $RELEASE_DIR"
echo "Проверьте: curl -fsS http://127.0.0.1:8092/api/health"
echo "Rollback: $SRC_DIR/deploy/rollback.sh <release_id>"
