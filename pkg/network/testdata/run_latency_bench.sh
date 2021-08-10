#!/bin/bash

set -e

if [ "${EUID}" != "0" ]
then
  echo "Error: this command must be run as root" >&2
  exit 1
fi

if ! [ -x "$(command -v benchstat)" ]
then
  echo "Error: benchstat could not be found" >&2
  exit 1
fi

now=$(date -u +"%Y-%m-%dT%H%M%S")
RESFILE=/tmp/ebpf-bench-results.${now}
STATFILE=/tmp/ebpf-bench-benchstat.${now}

# using the test.count flag doesn't let us report probe times for each iteration
# use bash loop instead
for _ in {1..5}
do
  go test \
    -mod=mod \
    -tags linux_bpf \
    ./pkg/network/ebpf/... \
    -bench BenchmarkTCPLatency \
    -count=1 \
    -run XXX \
    >> "${RESFILE}"
  go test \
    -mod=mod \
    -tags linux_bpf \
    ./pkg/network/ebpf/... \
    -bench BenchmarkUDPLatency \
    -count=1 \
    -run XXX \
    >> "${RESFILE}"
done

benchstat -sort name "${RESFILE}" > "${STATFILE}"
echo "${RESFILE}"
echo "${STATFILE}"
