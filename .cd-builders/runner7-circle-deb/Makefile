RUNTIMETAG := $(shell date +%Y%m%d)

build:
	docker build -t docker.io/stackstate/stackstate-agent-runner-circle:deb7_$(RUNTIMETAG) .
push:
	docker push docker.io/stackstate/stackstate-agent-runner-circle:deb7_$(RUNTIMETAG)

try:
	docker run -it docker.io/stackstate/stackstate-agent-runner-circle:deb7_$(RUNTIMETAG) sh

tag_latest:
	docker tag docker.io/stackstate/stackstate-agent-runner-circle:deb7_$(RUNTIMETAG) docker.io/stackstate/stackstate-agent-runner-circle:deb7_latest

push_latest:
	docker push docker.io/stackstate/stackstate-agent-runner-circle:deb7_latest
