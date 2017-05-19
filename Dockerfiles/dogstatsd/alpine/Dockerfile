FROM alpine:3.5

MAINTAINER Datadog <package@datadoghq.com>

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh /entrypoint.sh
COPY dogstatsd /dogstatsd

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "start"]
