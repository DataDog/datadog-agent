FROM alpine:3.10

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=true

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh probe.sh /
COPY dogstatsd.yaml /etc/datadog-agent/dogstatsd.yaml
COPY install_info /etc/datadog-agent/install_info
COPY static/dogstatsd /dogstatsd

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "-c", "/etc/datadog-agent/", "start"]
