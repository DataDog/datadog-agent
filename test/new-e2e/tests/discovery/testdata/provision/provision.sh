#!/usr/bin/env bash

set -e

apt-get update
apt-get install -y \
        ca-certificates \
        curl \
        gnupg \
        python3 \
        python3-pip \


# Install Python deps
pip install ddtrace

# Install Node
if [ ! -d "${HOME}/.nvm" ]; then
  curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh | bash
fi

export NVM_DIR="$HOME/.nvm"
# shellcheck source=/dev/null
source "${NVM_DIR}/nvm.sh"
# Retry a few times since occasional failures have been seen
nvm install 20 || nvm install 20 || nvm install 20

npm install json-server || npm install json-server
npm install /home/ubuntu/e2e-test/node/instrumented

# Install our own services
install_systemd_unit () {
  name=$1
  command=$2
  port=$3
  extraenv=$4

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
Environment="${extraenv}"

[Install]
WantedBy=multi-user.target
EOM
}

# Node
install_systemd_unit "node-json-server" "$NVM_DIR/nvm-exec npx json-server --port 8084 /home/ubuntu/e2e-test/node/json-server/db.json" "8084" ""
install_systemd_unit "node-instrumented" "$NVM_DIR/nvm-exec node /home/ubuntu/e2e-test/node/instrumented/server.js" "8085" ""

# Python
install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082" "DD_SERVICE=python-svc-dd"
install_systemd_unit "python-instrumented" "/usr/bin/python3 /home/ubuntu/e2e-test/python/instrumented.py" "8083" ""

systemctl daemon-reload

# leave them stopped
systemctl stop python-svc
