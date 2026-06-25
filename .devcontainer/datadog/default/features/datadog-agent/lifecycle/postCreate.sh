#!/bin/bash


# Start the developer environment
DEV_ENV_IMAGE=$(cat /opt/doghome/devcontainer/dev-env-image)
cd dd/datadog-agent && dda env dev start 2>&1 | tee "/home/bits/.dev-env-start.log"
