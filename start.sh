#!/usr/bin/env bash
# Linux/WSL: build and run DreamReel through systemd.
set -Eeuo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
ACTION="${1:-start}"
RUNTIME="$ROOT/.runtime"
export PATH="/usr/local/go/bin:/usr/lib/go-1.26/bin:$PATH"
mkdir -p "$RUNTIME"
exec 9>"$RUNTIME/deploy.lock"
flock -n 9 || { echo 'Another DreamReel operation is running.' >&2; exit 1; }
RUN_USER="$(id -un)"
if [[ "$EUID" == 0 ]]; then
  echo 'Run this script as the project owner; it requests sudo when needed.' >&2
  exit 1
fi

health() {
  for ((i=0; i<60; i++)); do
    if curl --noproxy '*' --fail --silent --max-time 3 http://127.0.0.1:8080/health >/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo 'API health check failed. See: ./start.sh logs' >&2
  return 1
}

has_tunnel() {
  systemctl cat dreamreel-frpc.service >/dev/null 2>&1
}

case "$ACTION" in
  deploy)
    for cmd in go npm curl python3 ffmpeg ffprobe; do
      command -v "$cmd" >/dev/null || { echo "Missing dependency: $cmd" >&2; exit 1; }
    done
    sudo -v
    mkdir -p "$RUNTIME/bin" "$RUNTIME/logs" "$ROOT/apps/api/uploads"
    echo 'Building and testing API / worker...'
    (
      cd "$ROOT/apps/api"
      go test ./internal/...
      go build -trimpath -ldflags='-s -w' -o "$RUNTIME/bin/dreamreel-api.next" ./cmd/dreamreel
      go build -trimpath -ldflags='-s -w' -o "$RUNTIME/bin/dreamreel-worker.next" ./cmd/worker
    )
    echo 'Building and testing web...'
    (
      cd "$ROOT/apps/web"
      npm ci --no-audit --no-fund
      npm test
      npm run build
    )
    mv -f "$RUNTIME/bin/dreamreel-api.next" "$RUNTIME/bin/dreamreel-api"
    mv -f "$RUNTIME/bin/dreamreel-worker.next" "$RUNTIME/bin/dreamreel-worker"
    sudo bash "$ROOT/deploy/system.sh" deploy "$ROOT" "$RUN_USER"
    health
    sudo systemctl start dreamreel-worker.service
    ;;
  start|restart)
    if [[ ! -x "$RUNTIME/bin/dreamreel-api" ]] || ! systemctl cat dreamreel-api.service >/dev/null 2>&1; then
      exec bash "$ROOT/start.sh" deploy 9>&-
    fi
    sudo bash "$ROOT/deploy/system.sh" "$ACTION" "$ROOT" "$RUN_USER"
    health
    sudo systemctl start dreamreel-worker.service
    ;;
  stop)
    # Shared MySQL / Redis / RabbitMQ / Nginx remain available to other projects.
    if has_tunnel; then sudo systemctl stop dreamreel-frpc.service; fi
    sudo systemctl stop dreamreel-worker.service dreamreel-api.service
    echo 'DreamReel API and worker stopped.'
    exit 0
    ;;
  status)
    systemctl --no-pager --full status dreamreel-api.service dreamreel-worker.service nginx.service
    if has_tunnel; then systemctl --no-pager --full status dreamreel-frpc.service || true; fi
    curl --noproxy '*' --fail --silent --show-error --max-time 5 http://127.0.0.1/health
    printf '\n'
    exit 0
    ;;
  logs)
    exec journalctl -u dreamreel-api.service -u dreamreel-worker.service -u dreamreel-frpc.service -n 100 -f
    ;;
  public-on)
    has_tunnel || { echo 'FRP is not installed. See docs/public-access.md.' >&2; exit 1; }
    health
    sudo systemctl enable --now dreamreel-frpc.service
    echo 'Public tunnel enabled: http://www.dreamreel.me/'
    exit 0
    ;;
  public-off)
    has_tunnel || { echo 'FRP is not installed.' >&2; exit 1; }
    sudo systemctl disable --now dreamreel-frpc.service
    echo 'Public tunnel stopped. Local access remains available.'
    exit 0
    ;;
  public-status)
    exec systemctl --no-pager --full status dreamreel-frpc.service
    ;;
  *)
    echo 'Usage: ./start.sh [start|deploy|restart|stop|status|logs|public-on|public-off|public-status]' >&2
    exit 2
    ;;
esac
if has_tunnel && systemctl is-enabled --quiet dreamreel-frpc.service; then
  sudo systemctl start dreamreel-frpc.service
fi
systemctl is-active --quiet dreamreel-worker.service
curl --noproxy '*' --fail --silent --show-error --max-time 5 http://127.0.0.1/health
printf '\nDreamReel is ready: http://%s/\n' "$(hostname -I | awk '{print $1}')"
if has_tunnel && systemctl is-active --quiet dreamreel-frpc.service; then
  echo 'Public entry: http://www.dreamreel.me/'
fi
