#!/bin/bash

# Don't allow starting without an apikey set
if [[ -z "${DD_API_KEY}" && ! -r "${DD_API_KEY_FILE}" ]]; then
    echo ""
    echo "========================================================================================================="
    echo "You must set either DD_API_KEY or DD_API_KEY_FILE environment variable to run the Datadog Agent container"
    echo "========================================================================================================="
    echo ""
    exit 1
fi
