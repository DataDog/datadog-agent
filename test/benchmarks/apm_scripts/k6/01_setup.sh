#!/bin/bash 
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
WORKDIR="$LOCAL_TEST_PREFIX/benchmark_runner"
# store the test to use between runs between different branches
mkdir -p k6 $WORKDIR/k6
cp $SCRIPT_DIR/../* $WORKDIR/
cp $SCRIPT_DIR/* $WORKDIR/k6

fetch_data_layer_from_registry.py registry.ddbuild.io ci/relenv-microbenchmarking-platform/ben-runner latest benrunner.tar.gz
tar xf benrunner.tar.gz 

./ben-runner -l debug execute -f ./test/benchmarks/apm_scripts/latency.yaml
exit 1
# fetch and unpack payloads
fetch_data_layer_from_registry.py registry.ddbuild.io apm-reliability-environment/trace-agent-payloads latest payloads.tar.gz
PAYLOADS_DIR="$WORKDIR/k6/payloads"
mkdir -p $PAYLOADS_DIR
tar xf payloads.tar.gz -C $PAYLOADS_DIR
rm payloads.tar.gz

ls -lah $PAYLOADS_DIR

