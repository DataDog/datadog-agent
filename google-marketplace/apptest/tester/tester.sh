#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

set -eo pipefail

export NAME="datadog--$(uuidgen)"
cat datadog-test-app.yaml.template | envsubst > datadog-test-app.yaml

cat datadog-test-app.yaml

kubectl apply -f datadog-test-app.yaml

