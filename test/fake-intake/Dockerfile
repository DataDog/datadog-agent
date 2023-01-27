## Based on https://docs.docker.com/language/golang/build-images/
# syntax=docker/dockerfile:1

## Build
FROM golang:1.19-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY fakeintake/*.go ./fakeintake/
COPY *.go ./

RUN go build -o /build

## Deploy
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=build /build /fakeintake

EXPOSE 5000

USER nonroot:nonroot

ENTRYPOINT ["/fakeintake"]
