## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1

## Build
FROM golang:1.21.5-alpine3.18 AS build

# need gcc to build with CGO_ENABLED=1
# need musl-dev to get stdlib.h
RUN apk add musl-dev gcc

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# need to explicitely run with CGO enabled
ENV CGO_ENABLED=1
RUN go build -o /fakeintake cmd/server/main.go

## Deploy
FROM alpine:3.18

RUN apk add curl

COPY --from=build /fakeintake /fakeintake

EXPOSE 80

ENTRYPOINT ["/fakeintake"]
