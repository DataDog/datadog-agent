FROM alpine:3.12

LABEL maintainer "Datadog <package@datadoghq.com>"

RUN apk add --no-cache socat

EXPOSE 8125/udp

CMD ["socat", "-s", "-u", "UDP-RECV:8125", "UNIX-SENDTO:/socket/statsd.socket"]
