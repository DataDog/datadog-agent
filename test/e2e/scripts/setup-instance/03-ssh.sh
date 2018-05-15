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

if [[ "${DATADOG_AGENT_IMAGE}x" == "x" ]]
then
    DATADOG_AGENT_IMAGE=datadog/agent-dev:master
    echo "Running outside the gitlab pipeline, setting DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
else
    # This is not elegant...
    echo "Running inside a gitlab pipeline, using DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
    eval "$(aws ecr get-login --region us-east-1 --no-include-email --registry-ids 486234852809)"
    scp -i id_rsa ${HOME}/.docker/config.json core@${MACHINE}:/home/core/.docker/config.json
fi

export DATADOG_AGENT_IMAGE

_ssh git clone https://github.com/DataDog/datadog-agent.git /home/core/datadog-agent
_ssh git -C /home/core/datadog-agent checkout ${COMMIT_ID}

_ssh timeout 180 /home/core/datadog-agent/test/e2e/scripts/run-instance/10-pupernetes-ready.sh

# Use a logged bash
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/20-argo-download.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/21-argo-setup.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/22-argo-submit.sh
_ssh_logged /home/core/datadog-agent/test/e2e/scripts/run-instance/23-argo-get.sh
