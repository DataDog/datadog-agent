RUNTIMETAG := $(shell date +%Y%m%d)

build_current:
	docker build -t stackstate/stackstate-agent-runner-gitlab:deb_$(RUNTIMETAG) .
push_current:
	docker push stackstate/stackstate-agent-runner-gitlab:deb_$(RUNTIMETAG)
tag_latest:
	docker tag stackstate/stackstate-agent-runner-gitlab:deb_$(RUNTIMETAG) stackstate/stackstate-agent-runner-gitlab:latest
push_latest:
	docker push stackstate/stackstate-agent-runner-gitlab:latest
