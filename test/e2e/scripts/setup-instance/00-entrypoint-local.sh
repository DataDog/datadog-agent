#!/usr/bin/env bash

printf '=%.0s' {0..79} ; echo
set -e
cd "$(dirname "$0")"

../run-instance/10-setup-kind.sh
../run-instance/11-setup-kind-cluster.sh
../run-instance/20-argo-download.sh
../run-instance/21-argo-setup.sh
../run-instance/22-argo-submit.sh
../run-instance/23-argo-get.sh
