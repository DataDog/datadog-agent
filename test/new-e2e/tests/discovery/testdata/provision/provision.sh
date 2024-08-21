#!/usr/bin/env bash

set -e

apt-get update
apt-get install -y ca-certificates curl gnupg python3

# Install our own services
install_systemd_unit () {
  name=$1
  command=$2
  port=$3

  cat > "/etc/systemd/system/${name}.service" <<- EOM
[Unit]
Description=${name}
After=network.target
StartLimitIntervalSec=0

[Service]
Type=simple
Restart=always
RestartSec=1
User=root
ExecStart=${command}
Environment="PORT=${port}"

[Install]
WantedBy=multi-user.target
EOM
}

install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082"

systemctl daemon-reload

# leave them stopped
systemctl stop python-svc
