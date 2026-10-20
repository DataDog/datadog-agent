#!/bin/sh
export HEAP_AGENT_PID=$$
/opt/datadog-agent/embedded/bin/python3 /etc/datadog-agent/capture_heap.py &
exec /opt/datadog-agent/bin/agent/agent run -c /etc/datadog-agent/datadog.yaml
