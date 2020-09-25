#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -ex

test $1 || {
    echo 'Must provide a ssh IP or DNS as $1'
    exit 1
}
MACHINE=$1

# get into the repo to grab the last commit hash
test ${COMMIT_ID} || {
    cd $(dirname $0)
    COMMIT_ID=$(git rev-parse --verify HEAD)
}

SSH_OPTS="-o ServerAliveInterval=20 -o ConnectTimeout=6 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${PWD}/id_rsa "
SEND_ENV="-o SendEnv DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE:-datadog/agent-dev:master}"

function _ssh() {
    ssh ${SSH_OPTS} -lcore ${MACHINE} $@
}

function _ssh_logged() {
    ssh ${SSH_OPTS} -lcore ${MACHINE} ${SEND_ENV} /bin/bash -l -c "$@"
}

until _ssh /bin/true
do
    sleep 5
done

until _ssh systemctl is-system-running
do
    sleep 5
done


_ssh git clone https://github.com/DataDog/datadog-agent.git /home/core/datadog-agent || {
    # To be able to retry
    echo "Already cloned, fetching new changes"
    _ssh git -C /home/core/datadog-agent fetch
}
_ssh git -C /home/core/datadog-agent checkout ${COMMIT_ID}

_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/10-pupernetes-wait.sh
_ssh timeout 120 /home/core/datadog-agent/test/e2e/scripts/run-instance/11-pupernetes-ready.sh

# Use a logged bash
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/20-argo-download.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/21-argo-setup.sh

_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/22-argo-submit.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/23-argo-get.sh
