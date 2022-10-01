#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set +x

test "$1" || {
    echo "Must provide a ssh IP or DNS as \$1"
    exit 1
}
MACHINE=$1

# get into the repo to grab the last commit hash
test "${COMMIT_ID}" || {
    cd "$(dirname "$0")"
    COMMIT_ID=$(git rev-parse --verify HEAD)
}

SSH_OPTS=("-o" "ServerAliveInterval=20" "-o" "ConnectTimeout=6" "-o" "StrictHostKeyChecking=no" "-o" "UserKnownHostsFile=/dev/null" "-i" "${PWD}/id_rsa" "-o" "SendEnv=CI_* DD_API_KEY DOCKER_REGISTRY_* DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE:-datadog/agent-dev:master} DATADOG_CLUSTER_AGENT_IMAGE=${DATADOG_CLUSTER_AGENT_IMAGE:-datadog/cluster-agent-dev:master} ARGO_WORKFLOW DATADOG_AGENT_SITE DATADOG_AGENT_API_KEY DATADOG_AGENT_APP_KEY")

function _ssh() {
    ssh "${SSH_OPTS[@]}" -lcore "${MACHINE}" "$@"
}

function _ssh_logged() {
    ssh "${SSH_OPTS[@]}" -lcore "${MACHINE}" /bin/bash -l -c "$@"
}

until _ssh /bin/true; do
    sleep 5
done

until _ssh systemctl is-system-running --wait; do
    if [[ $? -eq 255 ]]; then
        sleep 5
        continue
    fi
    _ssh journalctl -n 30 ||:
    _ssh LC_ALL=C systemctl list-units --failed ||:
    # Let's try to reboot until we have more clues about what can go wrong here.
    _ssh sudo systemctl reboot ||:
    sleep 5
done


_ssh git clone https://github.com/DataDog/datadog-agent.git /home/core/datadog-agent || {
    # To be able to retry
    echo "Already cloned, fetching new changes"
    _ssh git -C /home/core/datadog-agent fetch
}
_ssh git -C /home/core/datadog-agent checkout "${COMMIT_ID}"

if [[ -n ${DOCKER_REGISTRY_URL+x} ]] && [[ -n ${DOCKER_REGISTRY_LOGIN+x} ]] && [[ -n ${DOCKER_REGISTRY_PWD+x} ]]; then
    oldstate=$(shopt -po xtrace ||:); set +x  # Do not log credentials
    _ssh_logged \"sudo docker login --username \\\""${DOCKER_REGISTRY_LOGIN}"\\\" --password \\\""${DOCKER_REGISTRY_PWD}"\\\" \\\""${DOCKER_REGISTRY_URL}"\\\"\"
    eval "$oldstate"
else
    _ssh_logged \"sudo mkdir -p /root/.docker \&\& sudo touch /root/.docker/config.json\"
fi

_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/10-setup-kind.sh
# Force reconnection to take into account the addition of the `docker` group to the `core` user
_ssh -O exit ||:
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/11-setup-kind-cluster.sh

# Use a logged bash
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/20-argo-download.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/21-argo-setup.sh

_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/22-argo-submit.sh
set +e
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/23-argo-get.sh
EXIT_CODE=$?
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/24-argo-to-ci-setup.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/25-argo-to-ci.sh
set -e
export MACHINE
./04-send-dd-event.sh $EXIT_CODE
