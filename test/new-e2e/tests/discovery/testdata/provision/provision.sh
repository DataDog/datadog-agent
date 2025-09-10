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

  # Build environment variables section
  env_section="Environment=\"PORT=${port}\"\nEnvironment=\"NODE_VERSION=20\""

  # Split extraenv and add each variable as a separate Environment line
  if [[ -n "${extraenv}" ]]; then
    while IFS= read -r -d ' ' env_var; do
      if [[ -n "${env_var}" ]]; then
        env_section="${env_section}\nEnvironment=\"${env_var}\""
      fi
    done <<< "${extraenv} "
  fi

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
$(echo -e "${env_section}")
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
        sqlite3 \
        pkg-config \
        libyaml-dev \

gem install rails -v 7.1.5.1

## Create new Rails project
pushd /home/ubuntu
rails new rails-hello --minimal
popd

# Install our own services
## Node
install_systemd_unit "node-json-server" "$NVM_DIR/nvm-exec npx json-server --port 8084 /home/ubuntu/e2e-test/node/json-server/db.json" "8084" ""
install_systemd_unit "node-instrumented" "$NVM_DIR/nvm-exec node /home/ubuntu/e2e-test/node/instrumented/server.js" "8085" ""

## Python
install_systemd_unit "python-svc" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8082" "DD_SERVICE=python-svc-dd DD_VERSION=2.1 DD_ENV=prod"
install_systemd_unit "python-instrumented" "/usr/bin/python3 /home/ubuntu/e2e-test/python/instrumented.py" "8083" "DD_SERVICE=python-instrumented-dd"
install_systemd_unit "python-restricted" "/usr/bin/python3 /home/ubuntu/e2e-test/python/server.py" "8086" "DD_SERVICE=python-restricted-dd"

## Ruby
install_systemd_unit --workdir "/home/ubuntu/rails-hello" "rails-svc" "rails server" "7777" ""

systemctl daemon-reload

## leave them stopped
systemctl stop python-svc
