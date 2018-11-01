#!/bin/bash

# Don't allow starting without an apikey set
if [[ -z "${STS_API_KEY}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set an STS_API_KEY environment variable to run the StackState Agent container"
    echo "=================================================================================="
    echo ""
    exit 1
fi
