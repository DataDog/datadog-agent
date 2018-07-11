FROM alpine:3.6

LABEL maintainer "Datadog <package@datadoghq.com>"

RUN apk add --no-cache ca-certificates

ENV DOCKER_DD_AGENT=yes \
    PATH=/:$PATH

COPY entrypoint.sh probe.sh /
COPY static/dogstatsd /dogstatsd

EXPOSE 8125/udp

HEALTHCHECK --interval=2m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "start"]
