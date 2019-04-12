RUNTIMETAG := $(shell date +%Y%m%d)

build:
	docker build -t stackstate/stackstate-agent-runner-circle:deb_$(RUNTIMETAG) .
push:
	docker push stackstate/stackstate-agent-runner-circle:deb_$(RUNTIMETAG)

tag_latest:
	docker tag stackstate/stackstate-agent-runner-circle:deb_$(RUNTIMETAG) stackstate/stackstate-agent-runner-circle:deb_latest

push_latest:
	docker push stackstate/stackstate-agent-runner-circle:deb_latest
