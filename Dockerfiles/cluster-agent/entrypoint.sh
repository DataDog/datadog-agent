#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.


##### Core config #####

if [[ -z $DD_API_KEY ]]; then
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    exit 1
fi

if [ -z $DD_DD_URL ]; then
    export DD_DD_URL="https://app.datadoghq.com"
fi

chmod +x /opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent
sync	# Fix for 'Text file busy' error

##### Starting up #####
export PATH="/opt/datadog-cluster-agent/bin/datadog-cluster-agent/:/opt/datadog-cluster-agent/embedded/bin/":$PATH

exec "$@"
