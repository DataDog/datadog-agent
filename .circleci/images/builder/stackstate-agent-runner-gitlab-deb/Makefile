RUNTIMETAG := $(shell date +%Y%m%d)

build:
	docker build -t stackstate/stackstate-agent-runner-gitlab:latest .
push:
	docker push stackstate/stackstate-agent-runner-gitlab:latest
