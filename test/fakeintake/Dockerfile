## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1
ARG GO_BASE_IMAGE_REGISTRY=docker.io
ARG ALPINE_BASE_IMAGE_REGISTRY=docker.io
ARG GO_VERSION=1.26.6

## Build
FROM ${GO_BASE_IMAGE_REGISTRY}/library/golang:${GO_VERSION} AS build

# need gcc to build with CGO_ENABLED=1
# need musl-dev to get stdlib.h
ARG CI
# APT mirror resilience in CI. The golang base is Debian, so apt uses
# deb.debian.org (a CDN). We rewrite the deb822 URIs: to an ordered list with
# Debian's AWS/CloudFront mirror (cdn-aws.deb.debian.org) first and deb.debian.org
# as fallback, and bound each request with a short timeout + Retries 1 so a slow
# mirror becomes a fast error that fails over. (Debian has no EC2 archive mirror
# like Ubuntu's; cdn-aws is its AWS-region CDN.)
RUN if [ "$CI" = "true" ]; then \
  printf 'Acquire::Retries "1";\nAcquire::http::Timeout "10";\nAcquire::https::Timeout "10";\n' > /etc/apt/apt.conf.d/99datadog-mirror-resilience && \
  for f in /etc/apt/sources.list.d/debian.sources; do \
    if [ -f "$f" ]; then \
      sed -i -E \
        -e 's#^(URIs:[[:space:]]+)https?://deb\.debian\.org/debian/?[[:space:]]*$#\1http://cdn-aws.deb.debian.org/debian http://deb.debian.org/debian#' \
        -e 's#^(URIs:[[:space:]]+)https?://deb\.debian\.org/debian-security/?[[:space:]]*$#\1http://cdn-aws.deb.debian.org/debian-security http://deb.debian.org/debian-security#' \
        "$f"; \
    fi; \
  done; \
  fi
RUN apt update && apt install --no-install-recommends -y musl-dev gcc musl-tools && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY test/fakeintake/go.mod test/fakeintake/go.sum ./

# every datadog-agent module imported by the fakeintake has to be copied in the build image
COPY pkg/proto /pkg/proto
COPY comp/core/tagger/origindetection /comp/core/tagger/origindetection
COPY comp/netflow/payload /comp/netflow/payload
COPY pkg/metrics /pkg/metrics
COPY pkg/network/payload /pkg/network/payload
COPY pkg/networkpath/payload /pkg/networkpath/payload
COPY pkg/tagger/types /pkg/tagger/types
COPY pkg/trace /pkg/trace

RUN go mod download

COPY test/fakeintake .

# need to explicitely run with CGO enabled and link against musl
ENV CGO_ENABLED=1
ENV CC=musl-gcc
ARG CI
RUN go build -o /fakeintake cmd/server/main.go

## Deploy
FROM ${ALPINE_BASE_IMAGE_REGISTRY}/library/alpine:3.24.1

# --timeout (global flag, before the subcommand) bounds each network operation so
# a slow Alpine CDN fails fast instead of hanging the build. Default is 60s.
RUN apk --timeout 10 add curl

COPY --from=build /fakeintake /fakeintake

EXPOSE 80

ENTRYPOINT ["/fakeintake"]
