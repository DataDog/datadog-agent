ARG BASE_IMAGE_DD_VERSION=latest
FROM gcr.io/datadoghq/agent:$BASE_IMAGE_DD_VERSION AS baseimage

FROM baseimage

# copy otel requirements
COPY otel-agent /opt/datadog-agent/embedded/bin/
COPY otel-config.yaml /etc/datadog-agent/otel-config.yaml

COPY s6-services/otel /etc/services.d/otel

# Update entrypoints and what not
COPY entrypoint.d /opt/entrypoints
RUN chmod 755 -R /opt/entrypoints \
  && chown dd-agent:root /etc/datadog-agent/otel-config.yaml \
  && rm -rf /var/run && mkdir -p /var/run/s6 && mkdir -p /var/run/datadog \
  && chown -R dd-agent:root /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/ \
  && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/
