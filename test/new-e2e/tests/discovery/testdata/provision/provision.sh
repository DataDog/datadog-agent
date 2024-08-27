#!/usr/bin/env bash

set -e

apt-get update
apt-get install -y ca-certificates curl gnupg python3 python3-pip

pip install ddtrace

# Install Node
if [ ! -d "${HOME}/.nvm" ]; then
  curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh | bash
fi

export NVM_DIR="$HOME/.nvm"
# shellcheck source=/dev/null
source "${NVM_DIR}/nvm.sh"
nvm install 20

npm install json-server

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
Environment=NODE_VERSION=20

[Install]
WantedBy=multi-user.target
EOM
}

install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082"
install_systemd_unit "python-instrumented" "/usr/bin/python3 /home/ubuntu/e2e-test/python/instrumented.py" "8083"

install_systemd_unit "node-json-server" "$NVM_DIR/nvm-exec npx json-server --port 8084 /home/ubuntu/e2e-test/node/json-server/db.json" "8084"

systemctl daemon-reload

# leave them stopped
systemctl stop python-svc
