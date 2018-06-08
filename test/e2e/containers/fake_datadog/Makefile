TAG?=latest

.PHONY: venv pip build default push

default: pip

venv:
	virtualenv venv -p python3

pip: venv
	venv/bin/pip install -r app/requirements.txt

build:
	docker build --force-rm -t datadog/fake-datadog:$(TAG) .

push:
	docker push datadog/fake-datadog:$(TAG)
