#!/usr/bin/env bash

set -ex
cd $(dirname $0)

../run-instance/10-pupernetes-ready.sh
../run-instance/20-argo-download.sh
../run-instance/21-argo-setup.sh
../run-instance/22-argo-submit.sh
../run-instance/23-argo-get.sh
