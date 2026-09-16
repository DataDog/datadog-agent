#!/bin/bash

# Do not allow starting without authentication configured.
# Either DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID (Workload Identity Federation) must be set.
if [[ -z "${DD_API_KEY}" ]] && [[ -z "${DD_DELEGATED_AUTH_ORG_UUID}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set either a DD_API_KEY or DD_DELEGATED_AUTH_ORG_UUID environment"
    echo "variable to run the Datadog Agent container."
    echo "=================================================================================="
    echo ""
    exit 1
fi
