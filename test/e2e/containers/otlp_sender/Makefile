TAG?=latest

default: push

otlpsender:
	GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/sender

docker-build: otlpsender
	docker build --force-rm -t datadog/docker-library:e2e-otlp-sender_$(TAG) .

push: docker-build
	docker push datadog/docker-library:e2e-otlp-sender_$(TAG)
