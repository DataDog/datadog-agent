#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.


##### Core config #####

if [ -z $DD_API_KEY ]; then
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    exit 1
fi

if [ -z $DD_DD_URL ]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

if [ -z $DD_DOGSTATSD_SOCKET ]; then
    export DD_DOGSTATSD_NON_LOCAL_TRAFFIC=1
else
    if [ -e $DD_DOGSTATSD_SOCKET ]; then
        if [ -S $DD_DOGSTATSD_SOCKET ]; then
            echo "Deleting existing socket at ${DD_DOGSTATSD_SOCKET}"
            rm $DD_DOGSTATSD_SOCKET
        else
            echo "${DD_DOGSTATSD_SOCKET} exists and is not a socket, please check your volume options"
            exit 1
        fi
    fi
fi

# Copy custom confs

find /conf.d -name '*.yaml' -exec cp --parents {} /etc/datadog-agent/ \;

find /checks.d -name '*.py' -exec cp --parents {} /etc/datadog-agent/ \;

##### Starting up #####

exec "$@"
