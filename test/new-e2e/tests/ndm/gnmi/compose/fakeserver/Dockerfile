ARG DD_REGISTRY=669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub
FROM ${DD_REGISTRY}/library/golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /gnmi-fakeserver .

FROM ${DD_REGISTRY}/library/alpine:3.20.3
COPY --from=build /gnmi-fakeserver /usr/local/bin/gnmi-fakeserver
EXPOSE 57400
ENTRYPOINT ["/usr/local/bin/gnmi-fakeserver"]
