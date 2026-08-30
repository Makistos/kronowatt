#!/usr/bin/env bash
set -euo pipefail

# Builds the backend and frontend on the dev machine and ships the
# artifacts to the target box (Fujitsu Esprimo Q510) via rsync, then
# restarts the systemd services there. Run from the repo root.

TARGET_HOST="${KRONOWATT_HOST:?set KRONOWATT_HOST, e.g. kronowatt.local}"
TARGET_USER="${KRONOWATT_USER:-kronowatt}"
REMOTE_ROOT="/opt/kronowatt"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Building backend"
(cd "$repo_root/backend" && GOOS=linux GOARCH=amd64 go build -o build/kronowatt-server ./cmd/server)

echo "==> Building frontend"
(cd "$repo_root/frontend" && npm ci && npm run build)

echo "==> Syncing backend artifact to $TARGET_HOST"
rsync -avz --mkpath \
  "$repo_root/backend/build/kronowatt-server" \
  "$TARGET_USER@$TARGET_HOST:$REMOTE_ROOT/backend/kronowatt-server"

echo "==> Syncing frontend build to $TARGET_HOST"
rsync -avz --delete --mkpath \
  "$repo_root/frontend/build/" \
  "$TARGET_USER@$TARGET_HOST:$REMOTE_ROOT/frontend/"

echo "==> Restarting services on $TARGET_HOST"
ssh "$TARGET_USER@$TARGET_HOST" \
  "sudo systemctl restart kronowatt-backend kronowatt-frontend"

echo "==> Done"
