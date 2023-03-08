#!/bin/bash

fetch_data_layer_from_registry.py registry.ddbuild.io apm-reliability-environment/trace-agent-payloads latest payloads.tar.gz

tar xf payloads.tar.gz -C ./test/benchmarks/apm_scripts/payloads
rm payloads.tar.gz

ls -lah