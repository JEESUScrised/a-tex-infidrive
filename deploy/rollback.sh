#!/usr/bin/env bash
set -euo pipefail

OPT_ROOT=/opt/infidrive
SERVICE=infidrive
TARGET="${1:-}"

if [[ -z "$TARGET" ]]; then
  echo "usage: rollback.sh <release_id>" >&2
  echo "available:" >&2
  ls -1 "$OPT_ROOT/releases" >&2 || true
  exit 2
fi

RELEASE_DIR="$OPT_ROOT/releases/$TARGET"
if [[ ! -x "$RELEASE_DIR/bin/infidrive" ]]; then
  echo "release not found: $RELEASE_DIR" >&2
  exit 1
fi

ln -sfn "$RELEASE_DIR" "$OPT_ROOT/current"
systemctl restart "$SERVICE"
systemctl --no-pager --full status "$SERVICE" || true
echo "rolled back to $TARGET"
