ARG BASE_IMAGE_DD_VERSION=latest
FROM registry.datadoghq.com/agent:$BASE_IMAGE_DD_VERSION AS baseimage

FROM baseimage

# copy otel requirements
COPY otel-agent /opt/datadog-agent/embedded/bin/
COPY otel-config.yaml /etc/datadog-agent/otel-config.yaml

COPY s6-services/otel /etc/services.d/otel

# Update entrypoints and what not
COPY entrypoint.d /opt/entrypoints
RUN chmod 755 -R /opt/entrypoints \
  && chown dd-agent:root /etc/datadog-agent/otel-config.yaml \
  && rm -rf /var/run && mkdir -p /var/run/s6 && mkdir -p /var/run/datadog && mkdir -p /opt/datadog-agent/run \
  && chown -R dd-agent:root /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/ /opt/datadog-agent/run \
  && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/ /opt/datadog-agent/run

# Set owner-only (0500) perms on built-in Rust shared-library checks (no-op when none are present).
RUN find /etc/datadog-agent/checks.d -name 'libdatadog-agent-*.so' -exec chmod 0500 {} \;
