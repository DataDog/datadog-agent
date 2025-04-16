#!/bin/bash

sysprobe_cfg="/etc/datadog-agent/system-probe.yaml"

if [ -f "$sysprobe_cfg" ] && grep -Eq '^ *enable_tcp_queue_length *: *true' "$sysprobe_cfg" || [[ "$DD_SYSTEM_PROBE_CONFIG_ENABLE_TCP_QUEUE_LENGTH" == "true" ]]; then
  if [ -f /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.example ]; then
    mv /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.default
  fi
fi

if [ -f "$sysprobe_cfg" ] && grep -Eq '^ *enable_oom_kill *: *true' "$sysprobe_cfg" || [[ "$DD_SYSTEM_PROBE_CONFIG_ENABLE_OOM_KILL" == "true" ]]; then
  if [ -f /etc/datadog-agent/conf.d/oom_kill.d/conf.yaml.example ]; then
    mv /etc/datadog-agent/conf.d/oom_kill.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/oom_kill.d/conf.yaml.default
  fi
fi

# Match the key gpu_monitoring.enabled: true using Python's YAML parser, which is included in the base image
# and is more robust than using regexes.
if [ -f "$sysprobe_cfg" ]; then
  gpu_monitoring_enabled=$(python3 -c "import yaml, sys; data=yaml.safe_load(sys.stdin) or {}; print(bool(data.get('gpu_monitoring', {}).get('enabled')))" < "$sysprobe_cfg")
else
  gpu_monitoring_enabled="False"
fi

# Note gpu_monitoring_enabled is a Python boolean, so casing is important
if [[ "$gpu_monitoring_enabled" == "True" ]] || [[ "$DD_GPU_MONITORING_ENABLED" == "true" ]]; then
  if [ -f /etc/datadog-agent/conf.d/gpu.d/conf.yaml.example ]; then
    mv /etc/datadog-agent/conf.d/gpu.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/gpu.d/conf.yaml.default
  fi
fi
