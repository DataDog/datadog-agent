#!/bin/bash

# Don't allow starting without an apikey set
if [[ -z "${DD_API_KEY}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set an DD_API_KEY environment variable to run the Datadog Agent container"
    echo "=================================================================================="
    echo ""
    exit 1
fi
