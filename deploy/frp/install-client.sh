#!/usr/bin/env bash
# Run with sudo on target-wsl after copying the shared FRP token and public cert.
set -Eeuo pipefail
[[ "$EUID" == 0 ]] || { echo 'Root privileges required.' >&2; exit 1; }
ROOT="$(realpath -- "$1")"
OWNER="$2"
id "$OWNER" >/dev/null
[[ "$ROOT" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo 'Unsupported project path.' >&2; exit 1; }
SOURCE="$ROOT/.runtime/frp"
[[ -s "$SOURCE/auth.token" && -s "$SOURCE/server.crt" ]] || { echo 'FRP shared token / server certificate missing.' >&2; exit 1; }
curl --noproxy '*' -fsS --max-time 5 http://127.0.0.1/health >/dev/null
install -d -m 755 /opt/dreamreel-frp
install -m 755 "$ROOT/.runtime/tools/frp/frp_0.72.0_linux_amd64/frpc" /opt/dreamreel-frp/frpc
install -d -o "$OWNER" -g "$(id -gn "$OWNER")" -m 700 /etc/dreamreel-frp
install -o "$OWNER" -g "$(id -gn "$OWNER")" -m 600 "$SOURCE/auth.token" /etc/dreamreel-frp/auth.token
install -o "$OWNER" -g "$(id -gn "$OWNER")" -m 600 "$SOURCE/server.crt" /etc/dreamreel-frp/server.crt
cat > /etc/dreamreel-frp/frpc.toml <<'EOF'
serverAddr = "47.120.45.106"
serverPort = 7000
user = "dreamreel"
loginFailExit = false
auth.method = "token"
auth.additionalScopes = ["HeartBeats", "NewWorkConns"]
auth.tokenSource.type = "file"
auth.tokenSource.file.path = "/etc/dreamreel-frp/auth.token"
transport.tls.enable = true
transport.tls.trustedCaFile = "/etc/dreamreel-frp/server.crt"
transport.tls.serverName = "www.dreamreel.me"
log.to = "console"
log.level = "info"
log.disablePrintColor = true

[[proxies]]
name = "www"
type = "http"
localIP = "127.0.0.1"
localPort = 80
customDomains = ["www.dreamreel.me"]
healthCheck.type = "http"
healthCheck.path = "/health"
healthCheck.intervalSeconds = 15
healthCheck.timeoutSeconds = 5
healthCheck.maxFailed = 3
EOF
chown "$OWNER:$(id -gn "$OWNER")" /etc/dreamreel-frp/frpc.toml
chmod 600 /etc/dreamreel-frp/frpc.toml
/opt/dreamreel-frp/frpc verify -c /etc/dreamreel-frp/frpc.toml
cat > /etc/systemd/system/dreamreel-frpc.service <<EOF
[Unit]
Description=DreamReel FRP tunnel to public entry
Wants=network-online.target nginx.service dreamreel-api.service
After=network-online.target nginx.service dreamreel-api.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=$OWNER
Group=$(id -gn "$OWNER")
ExecStart=/opt/dreamreel-frp/frpc -c /etc/dreamreel-frp/frpc.toml
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now dreamreel-frpc.service
systemctl is-active dreamreel-frpc.service
