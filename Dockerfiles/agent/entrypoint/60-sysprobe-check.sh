#!/bin/bash

if grep -Eq '^ *enable_tcp_queue_length *: *true' /etc/datadog-agent/system-probe.yaml; then
    mv /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.example \
       /etc/datadog-agent/conf.d/tcp_queue_length.d/conf.yaml.default
fi
