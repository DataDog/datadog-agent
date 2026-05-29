#!/bin/bash

# Check if api_key is defined in datadog.yaml (including ENC[...] secret handles)
CONFIG_FILE="/etc/datadog-agent/datadog.yaml"
API_KEY_FROM_CONFIG=""

if [[ -f "$CONFIG_FILE" ]]; then
    # Extract api_key from config file, handling both plain and ENC[...] formats
    API_KEY_FROM_CONFIG=$(python3 2>/dev/null -c "
import yaml
print(yaml.safe_load(open('$CONFIG_FILE')).get('api_key') or '')  # null -> None -> ''
")
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
