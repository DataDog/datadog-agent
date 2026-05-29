#!/usr/bin/env bash
# Antithesis test command for the rotation-loss demonstration.
#
# The SUT (the rotation-demo binary, this container's entrypoint) continuously runs
# the rotation-under-backpressure experiment and emits the `Always` /
# `backpressure-no-rotation-loss` assertion itself. This driver simply lets a timeline
# run for a while so Antithesis injects faults (CPU throttling, thread pausing, etc.)
# while the SUT loop executes and evaluates the property. The bug is deterministic, so
# the failing `Always` assertion is reported regardless; the fault injection explores
# whether the loss magnitude varies under different schedules.
set -euo pipefail

echo "rotation-loss driver: observing SUT for 30s while faults are injected"
sleep 30
echo "rotation-loss driver: done"
