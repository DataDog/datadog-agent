#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

docker build -t argo-to-junit-helper:local ./argo-to-junit