TAG?=$(shell date '+%Y%m%d')

.PHONY: venv pip build default push multiarch

default: pip

venv:
	virtualenv venv -p python3

pip: venv
	venv/bin/pip install -r app/requirements.txt

build:
	docker build --force-rm -t datadog/fake-datadog:$(TAG) .

multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t datadog/fake-datadog:$(TAG) . --push

push:
	docker push datadog/fake-datadog:$(TAG)
