#!/bin/bash



# Run install tools
cd ~/dd/datadog-agent
dda inv install-tools 2>&1 | tee "/home/bits/.install-tools.log"
