#!/bin/bash


# Start the developer environment
cd ~/dd/datadog-agent
# Prepull the developer environment image
dda env dev start 2>&1 | tee "/home/bits/.dev-env-start.log"
dda env dev stop 2>&1 | tee "/home/bits/.dev-env-stop.log"
dda env dev remove 2>&1 | tee "/home/bits/.dev-env-remove.log"
# Restore the configuration so that it is initialized again on first connection, when Git info are available
dda config restore
