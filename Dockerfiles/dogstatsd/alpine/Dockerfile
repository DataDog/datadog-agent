ARG BASE_IMAGE_ALPINE_VERSION=3.23.3
ARG BASE_IMAGE_REGISTRY=public.ecr.aws/docker

FROM ${BASE_IMAGE_REGISTRY}/library/alpine:$BASE_IMAGE_ALPINE_VERSION AS release-base
ARG TARGETARCH
ARG BASE_IMAGE_ALPINE_VERSION

LABEL baseimage.os "alpine"
LABEL baseimage.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.base.name "alpine:${BASE_IMAGE_ALPINE_VERSION}"
LABEL org.opencontainers.image.title "Datadog Dogstatsd Agent"

# APK mirror resilience: apk tries repositories in listed order and falls over to
# the next on failure. Keep the default dl-cdn.alpinelinux.org CDN as primary and
# add mirror.clarkson.edu (closest to us-east-1, where we build) as a secondary,
# so a degraded CDN fails over instead of stalling the build.

RUN ALPINE_RELEASE=$(cat /etc/alpine-release | sed "s/^\([0-9][0-9]*\.[0-9][0-9]*\).*$/\1/") && \
  printf 'http://dl-cdn.alpinelinux.org/alpine/v%s/main\nhttp://dl-cdn.alpinelinux.org/alpine/v%s/community\nhttp://mirror.clarkson.edu/alpine/v%s/main\nhttp://mirror.clarkson.edu/alpine/v%s/community\n' \
    "${ALPINE_RELEASE}" "${ALPINE_RELEASE}" "${ALPINE_RELEASE}" "${ALPINE_RELEASE}" > /etc/apk/repositories

# --timeout (a global flag, before the subcommand) bounds each network operation
# so a slow mirror fails fast and apk moves to the next repository instead of
# hanging. Default is 60s.
RUN apk --timeout 10 add --no-cache ca-certificates

COPY entrypoint.sh probe.sh /
RUN chmod 755 /entrypoint.sh /probe.sh
COPY dogstatsd.yaml /etc/datadog-agent/dogstatsd.yaml
COPY install_info /etc/datadog-agent/install_info

FROM release-base

COPY --chmod=755 --from=artifacts static/dogstatsd.$TARGETARCH /dogstatsd

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA

ENV DOCKER_DD_AGENT=true \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

EXPOSE 8125/udp

HEALTHCHECK --interval=1m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/dogstatsd", "-c", "/etc/datadog-agent/", "start"]
