ARG BASE_IMAGE_ALPINE_VERSION=3.18
FROM public.ecr.aws/docker/library/alpine:$BASE_IMAGE_ALPINE_VERSION
ARG TARGETARCH
ARG BASE_IMAGE_ALPINE_VERSION
ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA

LABEL baseimage.os "alpine"
LABEL baseimage.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.base.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL org.opencontainers.image.title "Datadog Dogstatsd Agent"

ENV DOCKER_DD_AGENT=true \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

# We add mirror.clarkson.edu as a secondary mirror, as it's closest to us-east-1
# where we build the agent

RUN ALPINE_RELEASE=$(cat /etc/alpine-release | sed "s/^\(\d\+\.\d\+\).\+$/\1/") && \
  echo -e "http://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/main\nhttp://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/community" >> /etc/apk/repositories

RUN apk add --no-cache ca-certificates

COPY entrypoint.sh probe.sh /
RUN chmod 755 /entrypoint.sh /probe.sh
COPY dogstatsd.yaml /etc/datadog-agent/dogstatsd.yaml
COPY install_info /etc/datadog-agent/install_info
COPY static/dogstatsd.$TARGETARCH /dogstatsd

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "-c", "/etc/datadog-agent/", "start"]
