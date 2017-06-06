# Development Dockerfile
FROM golang:1.8.3

MAINTAINER Datadog <package@datadoghq.com>

WORKDIR /go/src/app
COPY . .

RUN go-wrapper download   # "go get -d -v ./..."

CMD exec /bin/bash -c "trap : TERM INT; sleep infinity & wait"
