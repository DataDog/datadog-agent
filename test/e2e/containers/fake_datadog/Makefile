TAG?=latest

.PHONY: venv pip build default

default: pip

venv:
	virtualenv venv -p python3

pip: venv
	venv/bin/pip install -r app/requirements.txt

build:
	docker build --force-rm -t julienbalestra/fake-datadog:$(TAG) .

push:
	docker push julienbalestra/fake-datadog:$(TAG)
