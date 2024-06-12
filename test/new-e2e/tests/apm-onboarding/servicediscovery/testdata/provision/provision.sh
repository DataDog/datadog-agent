#!/usr/bin/env bash

set -e

apt-get update
apt-get install -y ca-certificates curl gnupg python3

# Install Node
if [ ! -d "${HOME}/.nvm" ]; then
  curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
fi

echo $HOME

source "${HOME}/.nvm/nvm.sh"
nvm install 20

# Install Go
if [ ! -d "/usr/local/go" ]; then
    wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
    rm go1.21.0.linux-amd64.tar.gz
fi

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
Environment="NODE_VERSION=20"

[Install]
WantedBy=multi-user.target
EOM
}

install_systemd_unit "go-svc" "/usr/local/go/bin/go run /home/ubuntu/e2e-test/go/main.go" "8080"
install_systemd_unit "node-svc" "/root/.nvm/nvm-exec node /home/ubuntu/e2e-test/node/server.js" "8081"
install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082"

systemctl daemon-reload

# leave them stopped
systemctl stop go-svc
systemctl stop node-svc
systemctl stop python-svc
