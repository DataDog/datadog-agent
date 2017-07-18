#!/bin/bash

set -e

current_dir=`dirname "${BASH_SOURCE[0]}"`

$PYLAUNCHER_BIN -conf datadog.yaml -py datadog_agent.py -- -v -s
