FROM ubuntu:xenial

RUN apt-get update && apt-get install -y conntrack

COPY system-probe /usr/local/bin

CMD system-probe
