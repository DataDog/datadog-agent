## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1

## Build
FROM golang:1.24.4-alpine3.22 AS build

# need gcc to build with CGO_ENABLED=1
# need musl-dev to get stdlib.h
RUN apk add musl-dev gcc

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

RUN go mod download

COPY test/fakeintake .

# need to explicitely run with CGO enabled
ENV CGO_ENABLED=1
RUN go build -o /fakeintake cmd/server/main.go

## Deploy
FROM alpine:3.21

RUN apk add curl

COPY --from=build /fakeintake /fakeintake

EXPOSE 80

ENTRYPOINT ["/fakeintake"]
