#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.


##### Core config #####

if [[ -z $DD_API_KEY ]]; then
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    exit 1
fi

if [ -z $DD_DD_URL ]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

if [[ -z $DD_CMD_PORT ]]; then
    export DD_CMD_PORT=$DD_CMD_PORT
fi

##### Starting up #####

export PATH="/opt/datadog-agent/bin/datadog-cluster-agent/:/opt/datadog-agent/bin/:$PATH"

exec "$@"
