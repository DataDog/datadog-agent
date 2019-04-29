RUNTIMETAG := $(shell date +%Y%m%d)

build_current:
	docker build -t stackstate/stackstate-agent-runner-gitlab:centos6_$(RUNTIMETAG) .
push_current:
	docker push stackstate/stackstate-agent-runner-gitlab:centos6_$(RUNTIMETAG)

tag_centos6:
	docker tag stackstate/stackstate-agent-runner-gitlab:centos6_$(RUNTIMETAG) stackstate/stackstate-agent-runner-gitlab:centos6
push_centos6:
	docker push stackstate/stackstate-agent-runner-gitlab:centos6
