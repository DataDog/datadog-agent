#!/bin/bash


# Run install tools
cd ~/dd/datadog-agent

su bits --login 'dda inv install-tools 2>&1 | tee "/home/bits/.install-tools.log"'

