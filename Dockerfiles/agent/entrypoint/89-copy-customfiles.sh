#!/bin/bash

# Copy the custom checks and confs in the /etc/stackstate-agent folder
find /conf.d -name '*.yaml' -exec cp --parents -fv {} /etc/stackstate-agent/ \;
find /checks.d -name '*.py' -exec cp --parents -fv {} /etc/stackstate-agent/ \;
