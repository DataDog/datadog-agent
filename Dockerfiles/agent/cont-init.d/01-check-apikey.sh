#!/bin/bash

# Check if api_key is defined in datadog.yaml (including ENC[...] secret handles)
CONFIG_FILE="/etc/datadog-agent/datadog.yaml"
API_KEY_FROM_CONFIG=""

if [[ -f "$CONFIG_FILE" ]]; then
    # Extract api_key from config file, handling both plain and ENC[...] formats
    API_KEY_FROM_CONFIG=$(grep -E "^[[:space:]]*api_key[[:space:]]*:" "$CONFIG_FILE" | sed 's/^[[:space:]]*api_key[[:space:]]*:[[:space:]]*//' | sed 's/[[:space:]]*$//')
fi

# Don't allow starting without an apikey set
if [[ -z "${DD_API_KEY}" && -z "${API_KEY_FROM_CONFIG}" ]]; then
    echo ""
    echo "=================================================================================="
    echo "You must set either:"
    echo "  - DD_API_KEY environment variable, or"
    echo "  - api_key in /etc/datadog-agent/datadog.yaml (including ENC[...] secret handles)"
    echo "to run the Datadog Agent container"
    echo "=================================================================================="
    echo ""
    exit 1
fi
