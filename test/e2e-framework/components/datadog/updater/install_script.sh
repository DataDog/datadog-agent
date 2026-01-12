#!/bin/bash
# (C) Datadog, Inc. 2010-present
# All rights reserved
# Licensed under Apache-2.0 License (see LICENSE)
set -e

DD_API_KEY="${AGENT_CONFIG:-deadbeefdeadbeefdeadbeefdeadbeef}" \
DD_SITE="datadoghq.com" \
DD_INSTALLER="true" \
DD_REMOTE_UPDATES="true" \
DD_REMOTE_POLICIES="true" \
DD_INSTALLER_REGISTRY_URL_INSTALLER_PACKAGE="669783387624.dkr.ecr.us-east-1.amazonaws.com" \
DD_INSTALLER_REGISTRY_AUTH_INSTALLER_PACKAGE="ecr" \
DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_INSTALLER="pipeline-${DD_PIPELINE_ID}" \
DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE="669783387624.dkr.ecr.us-east-1.amazonaws.com" \
DD_INSTALLER_REGISTRY_AUTH_AGENT_PACKAGE="ecr" \
DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT="pipeline-${DD_PIPELINE_ID}" \
TESTING_KEYS_URL="keys.datadoghq.com" \
TESTING_APT_URL="apttesting.datad0g.com/datadog-agent/pipeline-${DD_PIPELINE_ID}-a7" \
TESTING_APT_REPO_VERSION="stable-$(uname -m | sed 's/aarch64/arm64/; s/amd64/x86_64/') 7" \
TESTING_YUM_URL="yumtesting.datad0g.com" \
TESTING_YUM_VERSION_PATH="testing/pipeline-${DD_PIPELINE_ID}-a7/7" \
bash -c "$(curl -L https://install.datadoghq.com/scripts/install_script_agent7.sh)"
