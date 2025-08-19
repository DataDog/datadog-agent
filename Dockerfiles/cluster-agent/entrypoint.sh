#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.


##### Core config #####

if [[ -z "$DD_API_KEY" ]]; then
    echo "You must set an DD_API_KEY environment variable to run the Datadog Cluster Agent container"
    exit 1
fi

##### Copy the custom confs removing any ".." folder in the paths #####
find /conf.d -name '*.yaml' -o -name '*.yaml.default' | sed -E "s#/\.\.[^/]+##" | xargs -I{} cp --parents -fv {} /etc/datadog-agent/

##### Starting up #####
export PATH="/opt/datadog-agent/bin/datadog-cluster-agent/:/opt/datadog-agent/embedded/bin/":$PATH

exec "$@"
