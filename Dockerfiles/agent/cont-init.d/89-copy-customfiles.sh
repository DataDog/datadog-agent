#!/bin/bash

# Copy the custom checks and confs in the /etc/datadog-agent folder
find -L /conf.d -name '*.yaml' -exec cp --parents -fv {} /etc/datadog-agent/ \;
find -L /checks.d -name '*.py' -exec cp --parents -fv {} /etc/datadog-agent/ \;
