IMAGE_VERSION := $(shell date +'%Y%m%d%H%M')
build:
	docker build -t docker.io/stackstate/stackstate-agent-runner-gitlab:centos6_latest_v7 .
push:
	docker tag docker.io/stackstate/stackstate-agent-runner-gitlab:centos6_latest_v7 docker.io/stackstate/stackstate-agent-runner-gitlab:centos6_$(IMAGE_VERSION)_v7
	docker push docker.io/stackstate/stackstate-agent-runner-gitlab:centos6_latest_v7
	docker push docker.io/stackstate/stackstate-agent-runner-gitlab:centos6_$(IMAGE_VERSION)_v7
