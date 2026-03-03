## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1
ARG BASE_IMAGE_REGISTRY=docker.io
ARG GO_VERSION=1.25.7

## Build
FROM ${BASE_IMAGE_REGISTRY}/library/golang:${GO_VERSION} AS build

# need gcc to build with CGO_ENABLED=1
# need musl-dev to get stdlib.h
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
FROM ${BASE_IMAGE_REGISTRY}/library/alpine:3.21.3

RUN apk add curl

COPY --from=build /fakeintake /fakeintake

EXPOSE 80

ENTRYPOINT ["/fakeintake"]
