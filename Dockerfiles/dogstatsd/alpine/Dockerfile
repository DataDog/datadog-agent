ARG BASE_IMAGE_ALPINE_VERSION=3.23.3
ARG BASE_IMAGE_REGISTRY=public.ecr.aws/docker
ARG S6_VERSION=v2.2.0.3

# Fetch and verify s6-overlay, used to supervise dogstatsd and the (optional)
# Agent Data Plane process inside the container.
FROM ${BASE_IMAGE_REGISTRY}/library/alpine:$BASE_IMAGE_ALPINE_VERSION AS s6
ARG TARGETARCH
ARG S6_VERSION
ARG GENERAL_ARTIFACTS_CACHE_BUCKET_URL
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${GENERAL_ARTIFACTS_CACHE_BUCKET_URL:+${GENERAL_ARTIFACTS_CACHE_BUCKET_URL}/s6-overlay}
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${JUST_CONTAINERS_DOWNLOAD_LOCATION:-https://github.com/just-containers/s6-overlay/releases/download}
RUN apk add --no-cache curl
RUN S6ARCH=$([ "$TARGETARCH" = "amd64" ] && echo "amd64" || echo "aarch64") && \
  curl -L ${JUST_CONTAINERS_DOWNLOAD_LOCATION}/${S6_VERSION}/s6-overlay-${S6ARCH}.tar.gz -o /tmp/s6.tgz
COPY s6.$TARGETARCH.sha256 /tmp/s6.sha256
# To recompute a hash for a specific version, run:
# curl -L https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-<arch>.tar.gz | sha256sum
RUN echo "$(cat /tmp/s6.sha256)  /tmp/s6.tgz" | sha256sum -c -

FROM ${BASE_IMAGE_REGISTRY}/library/alpine:$BASE_IMAGE_ALPINE_VERSION AS release-base
ARG TARGETARCH
ARG BASE_IMAGE_ALPINE_VERSION

LABEL baseimage.os "alpine"
LABEL baseimage.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.base.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL org.opencontainers.image.title "Datadog Dogstatsd Agent"

# We add mirror.clarkson.edu as a secondary mirror, as it's closest to us-east-1
# where we build the agent

RUN ALPINE_RELEASE=$(cat /etc/alpine-release | sed "s/^\(\d\+\.\d\+\).\+$/\1/") && \
  echo -e "http://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/main\nhttp://mirror.clarkson.edu/alpine/v${ALPINE_RELEASE}/community" >> /etc/apk/repositories

RUN apk add --no-cache ca-certificates

# Extract s6-overlay. The ./bin entries are extracted into /usr/bin to stay
# compatible with merged-/usr layouts.
COPY --from=s6 /tmp/s6.tgz /tmp/s6.tgz
RUN tar xzf /tmp/s6.tgz -C / --exclude="./bin" \
  && tar xzf /tmp/s6.tgz -C /usr ./bin \
  && rm /tmp/s6.tgz

COPY entrypoint.sh probe.sh initlog.sh /
RUN chmod 755 /entrypoint.sh /probe.sh /initlog.sh
COPY s6-services /etc/services.d/
COPY dogstatsd.yaml /etc/datadog-agent/dogstatsd.yaml
COPY install_info /etc/datadog-agent/install_info
RUN mkdir -p /var/log/datadog

FROM release-base

COPY --chmod=755 --from=artifacts static/dogstatsd.$TARGETARCH /dogstatsd
# Agent Data Plane binary, supervised by s6. The data-plane service exits
# immediately (and disables itself) when data_plane.enabled is false.
COPY --chmod=755 --from=artifacts agent-data-plane.$TARGETARCH /opt/datadog-dogstatsd/embedded/bin/agent-data-plane

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA

ENV DOCKER_DD_AGENT=true \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

# entrypoint.sh validates configuration, sets defaults, then hands off to the
# s6-overlay init (/init), which supervises dogstatsd and agent-data-plane.
ENTRYPOINT ["/entrypoint.sh"]
