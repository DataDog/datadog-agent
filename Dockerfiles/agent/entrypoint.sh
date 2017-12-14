#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.


##### Core config #####

if [[ -z "${DD_API_KEY}" ]]; then
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container" >&2
    exit 1
fi

if [[ -z "${DD_DD_URL}" ]]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

if [[ -z "${DD_DOGSTATSD_SOCKET}" ]]; then
    export DD_DOGSTATSD_NON_LOCAL_TRAFFIC=1
elif [[ -e "${DD_DOGSTATSD_SOCKET}" ]]; then
    if [[ -S "${DD_DOGSTATSD_SOCKET}" ]]; then
        echo "Deleting existing socket at ${DD_DOGSTATSD_SOCKET}"
        rm -v "${DD_DOGSTATSD_SOCKET}" || exit $?
    else
        echo "${DD_DOGSTATSD_SOCKET} exists and is not a socket, please check your volume options" >&2
        ls -l "${DD_DOGSTATSD_SOCKET}" >&2
        exit 1
    fi
fi

if [[ "${KUBERNETES_SERVICE_PORT}" ]]; then
    export KUBERNETES="yes"
fi

# Install default datadog.yaml
if [[ "${KUBERNETES}" ]]; then
    ln -s /etc/datadog-agent/datadog-kubernetes.yaml /etc/datadog-agent/datadog.yaml
elif [ $ECS ]; then
    ln -s /etc/datadog-agent/datadog-ecs.yaml /etc/datadog-agent/datadog.yaml
else
    ln -s /etc/datadog-agent/datadog-docker.yaml /etc/datadog-agent/datadog.yaml
fi

# Copy custom confs

find /conf.d -name '*.yaml' -exec cp --parents {} /etc/datadog-agent/ \;

find /checks.d -name '*.py' -exec cp --parents {} /etc/datadog-agent/ \;

##### Starting up #####

exec "$@"
