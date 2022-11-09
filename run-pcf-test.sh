#!/bin/sh

while true; do
    invoke test --targets="./pkg/util/cloudproviders/cloudfoundry" --skip-linters --race --no-cache
done
