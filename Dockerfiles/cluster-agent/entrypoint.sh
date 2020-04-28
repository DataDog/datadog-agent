#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2020 Datadog, Inc.


##### Core config #####

if [[ -z "$DD_API_KEY" && ! -r "$DD_API_KEY_FILE" ]]; then
    echo "You must set either DD_API_KEY or DD_API_KEY_FILE environment variable to run the Datadog Cluster Agent container"
    exit 1
fi

##### Copy the custom confs #####
find /conf.d -name '*.yaml' -exec cp --parents -fv {} /etc/datadog-agent/ \;

##### Starting up #####
export PATH="/opt/datadog-agent/bin/datadog-cluster-agent/:/opt/datadog-agent/embedded/bin/":$PATH

exec "$@"
