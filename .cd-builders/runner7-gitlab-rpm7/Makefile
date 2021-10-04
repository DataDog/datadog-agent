RUNTIMETAG := $(shell date +%Y%m%d)
build:
	docker build -t docker.io/stackstate/stackstate-agent-runner-gitlab:centos7_latest_v7 .
push:
	docker tag docker.io/stackstate/stackstate-agent-runner-gitlab:centos7_latest_v7 docker.io/stackstate/stackstate-agent-runner-gitlab:centos7_$(RUNTIMETAG)_v7
	docker push docker.io/stackstate/stackstate-agent-runner-gitlab:centos7_latest_v7
	docker push docker.io/stackstate/stackstate-agent-runner-gitlab:centos7_$(RUNTIMETAG)_v7
