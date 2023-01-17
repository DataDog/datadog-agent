FROM alpine:3.12

LABEL baseimage.os "alpine"
LABEL baseimage.name "alpine:3.12"
LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=true

# We add mirror.clarkson.edu as a secondary mirror, as it's closest to us-east-1
# where we build the agent

RUN ALPINE_RELEASE=$(cat /etc/alpine-release | sed "s/^\(\d\+\.\d\+\).\+$/\1/") && \
  echo -e "http://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/main\nhttp://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/community" >> /etc/apk/repositories

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh probe.sh /
RUN chmod 755 /entrypoint.sh /probe.sh
COPY dogstatsd.yaml /etc/datadog-agent/dogstatsd.yaml
COPY install_info /etc/datadog-agent/install_info
COPY static/dogstatsd /dogstatsd

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "-c", "/etc/datadog-agent/", "start"]
