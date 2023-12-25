all:
	cp ../../../bin/cws-instrumentation/cws-instrumentation .
	docker build -t spi/myapp --platform linux/amd64 . -f Dockerfile

run:
	docker run --network=host --privileged spi/myapp
