#!/bin/sh
# Prepare the ddot-ebpf Docker build context.
#
# Single source of truth for what goes into the image, shared by the
# docker_build_host_profiler_standalone_* CI jobs and the host-profiler.publish
# task. Usage: prepare-image-context.sh <binary-path> [context-dir]

set -eu

binary=$1
ctx=${2:-Dockerfiles/ddot-ebpf}

cp "$binary" "$ctx/host-profiler"
cp cmd/host-profiler/dist/host-profiler-config.yaml "$ctx/"
cp cmd/host-profiler/deploy/seccomp-profile.json "$ctx/"
jq '. + {flags: ["SECCOMP_FILTER_FLAG_LOG"]}' cmd/host-profiler/deploy/seccomp-profile.json >"$ctx/logging-seccomp.json"
