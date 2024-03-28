#!/bin/bash

# Check if the URL argument is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <URL>"
    exit 1
fi

# Infinite loop
while true; do
    # Curl the provided URL
    curl -sS "$1"
    # Sleep for 5 seconds before next curl
    sleep 1
done

