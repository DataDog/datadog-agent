#!/usr/bin/env bash

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

SSH_OPTS="-o ServerAliveInterval=10 -o ConnectTimeout=2 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i id_rsa "

function _ssh() {
    ssh ${SSH_OPTS} -lcore ${MACHINE} $@
}

function _ssh_logged() {
    ssh ${SSH_OPTS} -lcore ${MACHINE} /bin/bash -l -c "$@"
}

until _ssh /bin/true
do
    sleep 5
done

_ssh git clone https://github.com/DataDog/datadog-agent.git /home/core/datadog-agent
_ssh git -C /home/core/datadog-agent checkout ${COMMIT_ID}

_ssh timeout 180 /home/core/datadog-agent/test/e2e/scripts/local/10-pupernetes-ready.sh

# Use a logged bash
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/local/20-argo-download.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/local/21-argo-setup.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/local/22-argo-submit.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/local/23-argo-get.sh
