#!/bin/bash

if grep -Eq '^ *enable_tcp_queue_length *: *true' /etc/datadog-agent/system-probe.yaml || [[ "$DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH" == "true" ]]; then
    mv /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.default
fi

if grep -Eq '^ *enable_oom_kill *: *true' /etc/datadog-agent/system-probe.yaml || [[ "$DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL" == "true" ]]; then
    mv /etc/datadog-agent/conf.d/oom_kill.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/oom_kill.d/conf.yaml.default
fi
