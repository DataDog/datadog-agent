TAG?=latest

.PHONY: venv pip build default

default: pip

venv:
	virtualenv venv -p python3

pip: venv
	venv/bin/pip install -r app/requirements.txt

build:
	docker build --force-rm -t datadog/docker-library:fake_datadog_$(TAG) .

push:
	docker push datadog/docker-library:fake_datadog_$(TAG)
