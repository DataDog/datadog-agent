#!/usr/bin/env bash

set -e

install_systemd_unit() {
  while [[ $# -ge 2 ]]; do
    case $1 in
      --workdir)
        shift
        workdir="WorkingDirectory=$1"
        shift
        ;;
    *)
      break
      ;;
    esac
  done

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
${workdir}

[Install]
WantedBy=multi-user.target
EOM
}

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

# Install Ruby
## Packages
apt-get install -y \
        ruby \
        ruby-dev \
        ruby-rails \
        sqlite3 \

## Create new Rails project
pushd /home/ubuntu
rails new rails-hello --minimal
bundle install --gemfile=/home/ubuntu/rails-hello/Gemfile
popd

# Install our own services
## Node
install_systemd_unit "node-json-server" "$NVM_DIR/nvm-exec npx json-server --port 8084 /home/ubuntu/e2e-test/node/json-server/db.json" "8084" ""
install_systemd_unit "node-instrumented" "$NVM_DIR/nvm-exec node /home/ubuntu/e2e-test/node/instrumented/server.js" "8085" ""

## Python
install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082" "DD_SERVICE=python-svc-dd"
install_systemd_unit "python-instrumented" "/usr/bin/python3 /home/ubuntu/e2e-test/python/instrumented.py" "8083" ""

## Ruby
install_systemd_unit --workdir "/home/ubuntu/rails-hello" "rails-svc" "rails server" "7777" ""

systemctl daemon-reload

## leave them stopped
systemctl stop python-svc
