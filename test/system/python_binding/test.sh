#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

set -e

current_dir=`dirname "${BASH_SOURCE[0]}"`

$PYLAUNCHER_BIN -conf datadog.yaml -py datadog_agent.py -- -v
