FROM alpine:3.8

LABEL maintainer "Datadog <package@datadoghq.com>"

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh /entrypoint.sh
COPY static/dogstatsd /dogstatsd

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "start"]
