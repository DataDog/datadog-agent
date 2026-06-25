#!/bin/bash


# Start the developer environment
cd ~/dd/datadog-agent
dda env dev start 2>&1 | tee "/home/bits/.dev-env-start.log"
