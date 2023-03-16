#!/bin/bash 

fetch_data_layer_from_registry.py registry.ddbuild.io ci/relenv-microbenchmarking-platform/ben-runner latest benrunner.tar.gz
tar xf benrunner.tar.gz 

./ben-runner -l debug execute -f ./test/benchmarks/apm_scripts/latency.br.yaml --verbose-bash --disable-docker
