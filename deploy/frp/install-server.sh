#!/usr/bin/env bash
# Run on the ECS server as root. No existing network/firewall services are changed.
set -Eeuo pipefail
[[ "$EUID" == 0 ]] || { echo 'Root privileges required.' >&2; exit 1; }
ARCHIVE="${1:-/tmp/dreamreel-frp.tar.gz}"
VERSION=0.72.0
EXPECTED=daf719b32ca2778770bb92955e231a07414d754378b9af328aa2642862479804
[[ "$(uname -m)" == x86_64 ]] || { echo 'This package requires Linux x86_64.' >&2; exit 1; }
printf '%s  %s\n' "$EXPECTED" "$ARCHIVE" | sha256sum -c -
OWN_PID="$(systemctl show dreamreel-frps.service -p MainPID --value 2>/dev/null || true)"
for port in 80 7000; do
  LISTENER="$(ss -H -ltnp "sport = :$port")"
  if [[ -n "$LISTENER" ]] && { [[ -z "$OWN_PID" || "$OWN_PID" == 0 ]] || [[ "$LISTENER" != *"pid=$OWN_PID,"* ]]; }; then
    echo "Port $port is already occupied; leaving its service unchanged." >&2
    exit 1
  fi
done
if ! id dreamreel-frp >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin dreamreel-frp
fi
install -d -m 755 /opt/dreamreel-frp/releases
tar -xzf "$ARCHIVE" -C /opt/dreamreel-frp/releases
install -m 755 "/opt/dreamreel-frp/releases/frp_${VERSION}_linux_amd64/frps" /opt/dreamreel-frp/frps
install -d -o root -g dreamreel-frp -m 750 /etc/dreamreel-frp
systemctl list-units --type=service --all --no-pager --no-legend 'easytier*' | awk '{print $1}' |
  while read -r unit; do systemctl show "$unit" -p Id -p ActiveState -p MainPID -p NRestarts; done > /opt/dreamreel-frp/easytier-before.status
if [[ ! -s /etc/dreamreel-frp/auth.token ]]; then
  (umask 077; openssl rand -hex 32 > /etc/dreamreel-frp/auth.token)
fi
if [[ ! -s /etc/dreamreel-frp/server.crt || ! -s /etc/dreamreel-frp/server.key ]]; then
  openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 365 \
    -subj '/CN=www.dreamreel.me' \
    -addext 'subjectAltName=DNS:www.dreamreel.me,IP:47.120.45.106' \
    -keyout /etc/dreamreel-frp/server.key -out /etc/dreamreel-frp/server.crt 2>/dev/null
fi
chown root:dreamreel-frp /etc/dreamreel-frp/{auth.token,server.key,server.crt}
chmod 640 /etc/dreamreel-frp/{auth.token,server.key,server.crt}
cat > /etc/dreamreel-frp/frps.toml <<'EOF'
bindAddr = "0.0.0.0"
bindPort = 7000
vhostHTTPPort = 80
vhostHTTPTimeout = 300
auth.method = "token"
auth.additionalScopes = ["HeartBeats", "NewWorkConns"]
auth.tokenSource.type = "file"
auth.tokenSource.file.path = "/etc/dreamreel-frp/auth.token"
transport.tls.force = true
transport.tls.certFile = "/etc/dreamreel-frp/server.crt"
transport.tls.keyFile = "/etc/dreamreel-frp/server.key"
log.to = "console"
log.level = "info"
log.disablePrintColor = true
EOF
chown root:dreamreel-frp /etc/dreamreel-frp/frps.toml
chmod 640 /etc/dreamreel-frp/frps.toml
/opt/dreamreel-frp/frps verify -c /etc/dreamreel-frp/frps.toml
cat > /etc/systemd/system/dreamreel-frps.service <<'EOF'
[Unit]
Description=DreamReel FRP public entry
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
User=dreamreel-frp
Group=dreamreel-frp
ExecStart=/opt/dreamreel-frp/frps -c /etc/dreamreel-frp/frps.toml
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now dreamreel-frps.service
systemctl is-active dreamreel-frps.service
ss -ltnp '( sport = :80 or sport = :7000 )'
