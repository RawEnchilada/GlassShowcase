#!/bin/sh
set -eu

SERVICE_NAME=${SERVICE_NAME:-glass-tower}
APP_DIR=${APP_DIR:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)}
APP_USER=${APP_USER:-$(id -un)}
APP_GROUP=${APP_GROUP:-$(id -gn)}
PORT=${PORT:-4300}
FRONTEND_ORIGIN=${FRONTEND_ORIGIN:-}
RUN_TESTS=${RUN_TESTS:-1}
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

if command -v sudo >/dev/null 2>&1 && [ "$(id -u)" -ne 0 ]; then
  SUDO=sudo
else
  SUDO=
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required on the Raspberry Pi." >&2
  exit 1
fi

cd "$APP_DIR"

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git fetch --all --prune
  if git symbolic-ref --quiet --short HEAD >/dev/null 2>&1; then
    git pull --ff-only
  else
    echo "Repository is in detached HEAD; fetched updates but skipped pull." >&2
  fi
fi

mkdir -p data
make

if [ "$RUN_TESTS" != "0" ]; then
  make test
fi

tmp_unit=$(mktemp)
trap 'rm -f "$tmp_unit"' EXIT

cat >"$tmp_unit" <<EOF
[Unit]
Description=Glass Tower rating server
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_GROUP}
WorkingDirectory=${APP_DIR}
Environment=PORT=${PORT}
Environment=FRONTEND_ORIGIN=${FRONTEND_ORIGIN}
ExecStart=${APP_DIR}/glass-tower
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=false
ReadWritePaths=${APP_DIR}/data

[Install]
WantedBy=multi-user.target
EOF

if [ ! -f "$UNIT_PATH" ] || ! cmp -s "$tmp_unit" "$UNIT_PATH"; then
  $SUDO install -m 0644 "$tmp_unit" "$UNIT_PATH"
  $SUDO systemctl daemon-reload
else
  echo "Systemd unit is already up to date."
fi

$SUDO systemctl enable "$SERVICE_NAME"
$SUDO systemctl restart "$SERVICE_NAME"
$SUDO systemctl --no-pager --full status "$SERVICE_NAME"
