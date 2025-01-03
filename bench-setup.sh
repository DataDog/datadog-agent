#!/bin/bash

set -euo pipefail

if [ -z "$DD_API_KEY" ]; then echo "DD_API_KEY variable is required"; exit 1; fi
if [ -z "$PIPELINE_ID" ]; then echo "PIPELINE_ID variable is required"; exit 1; fi

# install the agent package but do not start it
TESTING_APT_URL=apttesting.datad0g.com TESTING_APT_REPO_VERSION="pipeline-$PIPELINE_ID-a7-x86_64 7" \
    DD_SITE="datadoghq.com" DD_INSTALL_ONLY=1 \
    bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"

# set agent configuration
sudo tee /etc/datadog-agent/datadog.yaml <<- EOM
api_key: $DD_API_KEY

process_config:
    process_collection:
        enabled: true

apm_config:
    enabled: false
EOM

# and start the agent
sudo systemctl start datadog-agent
