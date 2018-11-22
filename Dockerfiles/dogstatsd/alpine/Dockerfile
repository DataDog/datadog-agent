FROM alpine:3.8

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=true \
    DD_HEALTH_PORT=5555

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh probe.sh /
COPY static/dogstatsd /dogstatsd

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "start"]
