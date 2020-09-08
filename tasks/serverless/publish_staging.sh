# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2020 Datadog, Inc.

#!/bin/bash
set -e

./scripts/build_layers.sh
./scripts/publish_layers.sh us-east-1
