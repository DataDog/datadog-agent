TAG?=latest

default: build push

build:
	docker build --force-rm -t datadog/docker-library:e2e-dsd-sender_$(TAG) .

push:
	docker push datadog/docker-library:e2e-dsd-sender_$(TAG)
