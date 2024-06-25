#!/bin/bash

if [ -f /opt/datadog-agent/embedded/bin/otel-agent ]; then
  if grep -Pzq '(?s)otelcollector(\s*):(\s*)enabled(\s*):(\s*)true' /etc/datadog-agent/datadog.yaml || [[ "$DD_OTEL_ENABLED" == "true" ]]; then
    if [ ! -f /etc/datadog-agent/otel-config.yaml ] && [ -f /etc/datadog-agent/otel-config.yaml.example ]; then
      mv /etc/datadog-agent/otel-config.yaml.example \
         /etc/datadog-agent/otel-config.yaml
    fi
    s6-hiercopy /etc/services-available.d/otel /var/run/s6/services/otel
  fi
fi

s6-svscanctl -a /var/run/s6/services

