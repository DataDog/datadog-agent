# syntax=docker/dockerfile:1
# Build context: test/fakeopms/ (run: docker build -t fakeopms:e2e .)
ARG GO_BASE_IMAGE_REGISTRY=docker.io
ARG ALPINE_BASE_IMAGE_REGISTRY=docker.io
ARG GO_VERSION=1.25.0

## Build
FROM ${GO_BASE_IMAGE_REGISTRY}/library/golang:${GO_VERSION} AS build

WORKDIR /app

COPY go.mod ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /fakeopms cmd/server/main.go

## Deploy
FROM ${ALPINE_BASE_IMAGE_REGISTRY}/library/alpine:3.23.3

COPY --from=build /fakeopms /fakeopms

EXPOSE 8080

ENTRYPOINT ["/fakeopms"]
