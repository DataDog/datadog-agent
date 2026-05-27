#!/bin/bash

# Don't allow starting without an apikey set
# Skip the check if Workload Identity Federation (cloud-based auth) is configured --
# DD_DELEGATED_AUTH_ORG_UUID replaces the need for a static API key.
if [[ -n "${DD_DELEGATED_AUTH_ORG_UUID}" ]]; then
    exit 0
fi

if [[ -z "${DD_API_KEY}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    echo "=================================================================================="
    echo ""
    exit 1
fi
