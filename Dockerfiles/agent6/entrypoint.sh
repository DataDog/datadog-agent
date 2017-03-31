#!/bin/bash
#set -e


##### Core config #####

if [[ -z $DD_API_KEY ]]; then
    echo "You must set an API_KEY environment variable to run the Datadog Agent container"
    exit 1
fi

##### Starting up #####

export PATH="/opt/datadog-agent6/bin/agent/:/opt/datadog-agent6/bin/:$PATH"

exec "$@"
