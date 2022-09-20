#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

docker build -t argo-to-junit-helper:local ./argo-to-junit

sudo curl -L --fail "https://github.com/DataDog/datadog-ci/releases/latest/download/datadog-ci_linux-x64" --output "/usr/local/bin/datadog-ci" && sudo chmod +x /usr/local/bin/datadog-ci
