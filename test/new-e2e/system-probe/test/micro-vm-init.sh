#!/bin/bash
set -eEuo pipefail

docker_dir=/kmt-dockers

# Add provisioning steps here !
## Start docker if available, some images (e.g. SUSE arm64 for CWS) do not have it installed
if command -v docker ; then
    systemctl start docker

    ## Load docker images
    if [[ -d "${docker_dir}" ]]; then
        find "${docker_dir}" -maxdepth 1 -type f -exec docker load -i {} \;
    fi
else
    echo "Docker not available, skipping docker provisioning"
fi

## Increase tracing buffer size a tad, the default is 64KiB.
if [[ -f /sys/kernel/debug/tracing/buffer_size_kb ]]; then
    echo "Setting tracing buffer size to 1024 KB"
    echo 1024 > /sys/kernel/debug/tracing/buffer_size_kb || \
        echo "Failed to set tracing buffer size, continuing anyway"
else
    echo "Tracing buffer size file not found, skipping"
fi

## Use expedited RCU grace periods.
##
## Detaching a kprobe has to wait for an RCU grace period before the handler can be freed.
## Normal RCU waits passively for every CPU to pass through a quiescent state, which costs tens
## of milliseconds; the test suites detach a few hundred probes each time they rebuild their eBPF
## manager, so that latency multiplies into whole minutes. Expediting trades IPI noise and power
## for grace periods that are orders of magnitude shorter, which is the right trade for a
## disposable test VM (and the wrong one for a real host, hence: KMT only, never the agent).
##
## Measured on the CWS functional tests: probe detach drops from 7.11s to 0.69s per rebuild on
## kernel 5.15, and from 34.28s to 20.02s on 6.12. See CWS_KMT_CI_SPEED.md.
## rcu_normal has to be cleared as well because it takes precedence over rcu_expedited.
for rcu_knob in rcu_normal:0 rcu_expedited:1; do
    rcu_path="/sys/kernel/${rcu_knob%%:*}"
    rcu_value="${rcu_knob##*:}"
    if [[ -f "${rcu_path}" ]]; then
        echo "Setting ${rcu_path} to ${rcu_value}"
        echo "${rcu_value}" > "${rcu_path}" || \
            echo "Failed to set ${rcu_path}, continuing anyway"
    else
        echo "${rcu_path} not found, skipping"
    fi
done

# VM provisioning end !

# Start tests
code=0

/opt/testing-tools/test-runner "$@" || code=$?

if [[ -f "/job_env.txt" ]]; then
    cp /job_env.txt /ci-visibility/junit/
else
    echo "job_env.txt not found. Continuing without it."
fi

tar -C /ci-visibility/testjson -czvf /ci-visibility/testjson.tar.gz .
tar -C /ci-visibility/junit -czvf /ci-visibility/junit.tar.gz .

if [ "${COLLECT_COMPLEXITY:-}" = "yes" ]; then
    echo "Collecting complexity data..."
    mkdir -p /verifier-complexity
    arch=$(uname -m)
    if [[ "${arch}" == "aarch64" ]]; then
        arch="arm64"
    fi

    test_root=$(echo "$@" | sed 's/.*-test-root \([^ ]*\).*/\1/')
    export DD_SYSTEM_PROBE_BPF_DIR="${test_root}/pkg/ebpf/bytecode/build/${arch}"

    # Set the value of COMPLEXITY_CALC_MAX_MEM_MB to 6000 if not set by the environment
    if [[ -z "${COMPLEXITY_CALC_MAX_MEM_MB:-}" ]]; then
        export COMPLEXITY_CALC_MAX_MEM_MB=6000
    fi

    # Limit maximum memory usage of the calculator to avoid OOM errors affecting the entire connector
    ulimit -v $((COMPLEXITY_CALC_MAX_MEM_MB * 1024))

    # The debug.SetMemoryLimit function we use in the calculator for memory
    # limits only takes into account memory managed by the Go runtime, so we
    # tell it to try to keep a smaller memory limit than the one enforced by
    # ulimit, to reduce the chances of going above that hard limit.
    COMPLEXITY_GO_MAX_MEM_LIMIT=$((COMPLEXITY_CALC_MAX_MEM_MB * 95 / 100))

    if /opt/testing-tools/verifier-calculator -line-complexity -complexity-data-dir /verifier-complexity/complexity-data  -summary-output /verifier-complexity/verifier_stats.json -memory-limit-mb "${COMPLEXITY_GO_MAX_MEM_LIMIT}" &> /verifier-complexity/calculator.log ; then
        echo "Data collected, creating tarball at /verifier-complexity.tar.gz"
        tar -C /verifier-complexity -czf /verifier-complexity.tar.gz . || echo "Failed to created verifier-complexity.tar.gz"
    else
        echo "Failed to collect complexity data"
        echo "Calculator log:"
        cat /verifier-complexity/calculator.log
    fi
fi

exit ${code}
