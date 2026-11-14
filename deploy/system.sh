#!/usr/bin/env bash
# Called by start.sh with sudo after builds finish.
set -Eeuo pipefail
[[ "$EUID" == 0 ]] || { echo 'Root privileges required.' >&2; exit 1; }
ACTION="$1"
ROOT="$(realpath -- "$2")"
RUN_USER="$3"
id "$RUN_USER" >/dev/null
[[ "$ROOT" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo 'Unsupported project path.' >&2; exit 1; }
systemctl start mysql.service redis-server.service rabbitmq-server.service

if [[ "$ACTION" == deploy ]]; then
  python3 "$ROOT/deploy/configure.py" "$ROOT" "$RUN_USER"
  for name in api worker; do
    AFTER='network-online.target mysql.service redis-server.service rabbitmq-server.service'
    [[ "$name" == worker ]] && AFTER="$AFTER dreamreel-api.service"
    cat >"/etc/systemd/system/dreamreel-$name.service" <<EOF
[Unit]
Description=DreamReel $name
Wants=network-online.target
After=$AFTER
StartLimitIntervalSec=0

[Service]
Type=simple
User=$RUN_USER
Group=$(id -gn "$RUN_USER")
WorkingDirectory=$ROOT/apps/api
ExecStart=$ROOT/.runtime/bin/dreamreel-$name -config $ROOT/.runtime/config.yaml
Restart=on-failure
RestartSec=3
TimeoutStopSec=20
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
Environment=GIN_MODE=release

[Install]
WantedBy=multi-user.target
EOF
  done
  # Nginx serves a public copy, so the user's home directory remains private.
  mkdir -p /var/www/dreamreel
  cp -a "$ROOT/apps/web/dist/." /var/www/dreamreel/
  find /var/www/dreamreel -type d -exec chmod 755 {} +
  find /var/www/dreamreel -type f -exec chmod 644 {} +
  install -m 644 "$ROOT/deploy/nginx.conf" /etc/nginx/sites-available/dreamreel
  # Disable the packaged welcome page, retaining its original configuration.
  if [[ -L /etc/nginx/sites-enabled/default ]] && [[ "$(readlink -f /etc/nginx/sites-enabled/default)" == /etc/nginx/sites-available/default ]]; then
    unlink /etc/nginx/sites-enabled/default
  fi
  ln -sfn /etc/nginx/sites-available/dreamreel /etc/nginx/sites-enabled/dreamreel
  nginx -t
  systemctl daemon-reload
  systemctl enable dreamreel-api.service dreamreel-worker.service nginx.service
fi
case "$ACTION" in
  deploy|restart)
    systemctl stop dreamreel-worker.service
    systemctl restart dreamreel-api.service
    ;;
  start) systemctl start dreamreel-api.service ;;
  *) echo 'Unsupported system action.' >&2; exit 2 ;;
esac
systemctl start nginx.service
systemctl reload nginx.service
