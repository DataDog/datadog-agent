## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1

## Build
FROM golang:1.20.11-alpine3.18 AS build

# need gcc to build with CGO_ENABLED=1
# need musl-dev to get stdlib.h
RUN apk add musl-dev gcc

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY server/*.go ./server/
COPY server/serverstore/*.go ./server/serverstore/
COPY api/*.go ./api/
COPY app/*.go ./app/
COPY aggregator/*.go ./aggregator/

# need to explicitely run with CGO enabled
RUN CGO_ENABLED=1 go build -o /build app/main.go

## Deploy
FROM alpine:3.18

WORKDIR /

RUN apk add curl

COPY --from=build /build /fakeintake

EXPOSE 80

ENTRYPOINT ["/fakeintake"]
