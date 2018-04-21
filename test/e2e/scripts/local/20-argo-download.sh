#!/bin/bash

set -ex

cd $(dirname $0)

curl -sLf https://github.com/argoproj/argo/releases/download/v2.1.0-beta2/argo-linux-amd64 -o argo
sha512sum -c argo.sha512sum

chmod +x ./argo
