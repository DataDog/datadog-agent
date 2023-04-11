#!/bin/bash 
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

fetch_data_layer_from_registry.py registry.ddbuild.io ci/relenv-microbenchmarking-platform/ben-runner latest-nightly benrunner.tar.gz
tar xf benrunner.tar.gz 

./ben-runner -l debug execute -f ./test/benchmarks/apm_scripts/latency.yaml
