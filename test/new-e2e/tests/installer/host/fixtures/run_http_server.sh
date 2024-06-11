#!/bin/bash

export DD_APM_INSTRUMENTATION_DEBUG=true
export DD_TRACE_DEBUG=true

# Start Python HTTP server in the background within a subshell
python3 /opt/fixtures/http_server.py >/tmp/server.log 2>&1 &
PID=$!
disown $PID

while ! curl -s http://localhost:8080 > /dev/null; do
    sleep 1
done
