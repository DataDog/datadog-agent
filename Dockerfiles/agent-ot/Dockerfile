ARG BASE_IMAGE_DD_VERSION=latest
FROM docker.io/datadog/agent:$BASE_IMAGE_DD_VERSION AS baseimage

ARG CIBUILD

ENV DEBIAN_FRONTEND=noninteractive

FROM baseimage

# copy otel requirements
COPY otel-agent /opt/datadog-agent/embedded/bin/
COPY otel-config.yaml /etc/datadog-agent/otel-config.yaml.example

RUN mkdir -p /etc/services-available.d/
COPY s6-services-opt /etc/services-available.d/
COPY cont-init.d /etc/cont-init.d/


# Update entrypoints and what not
COPY entrypoint.sh /bin/entrypoint.sh
COPY entrypoint.d /opt/entrypoints
RUN chmod 755 /bin/entrypoint.sh \
  && chmod 755 -R /opt/entrypoints \
  && chown dd-agent:root /etc/datadog-agent/otel-config.yaml.example

# RUN chown dd-agent:dd-agent /opt/datadog-agent/embedded/bin/otel-agent

CMD ["/bin/entrypoint.sh"]
