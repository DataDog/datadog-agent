RUNTIMETAG := $(shell date +%Y%m%d)

build:
	docker build -t docker.io/stackstate/stackstate-agent-runner-circle:centos6_$(RUNTIMETAG)_v7 .
push:
	docker push docker.io/stackstate/stackstate-agent-runner-circle:centos6_$(RUNTIMETAG)_v7

tag_latest:
	docker tag docker.io/stackstate/stackstate-agent-runner-circle:centos6_$(RUNTIMETAG)_v7 docker.io/stackstate/stackstate-agent-runner-circle:centos6_latest_v7

push_latest:
	docker push docker.io/stackstate/stackstate-agent-runner-circle:centos6_latest_v7

