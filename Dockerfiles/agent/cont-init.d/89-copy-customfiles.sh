#!/bin/bash

# Copy the custom checks and confs in the /etc/datadog-agent folder
find /conf.d -name '*.yaml' -exec cp --parents -fv {} /etc/datadog-agent/ \;
find /checks.d -name '*.py' -exec cp --parents -fv {} /etc/datadog-agent/ \;
