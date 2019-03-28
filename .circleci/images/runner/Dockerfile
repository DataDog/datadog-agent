FROM ubuntu:18.04

# Pre-requisites
RUN set -ex \
    && apt-get update && apt-get install -y --no-install-recommends \
        gnupg ca-certificates \
        gcc g++ make git ssh curl \
        python-dev python3.7-dev python3-distutils \
        libssl-dev libsnmp-base libsnmp-dev libpq-dev snmp-mibs-downloader libsystemd-dev

ENV GIMME_GO_VERSION 1.11.5

# Gimme
RUN curl -sL -o /bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme
RUN chmod +x /bin/gimme
RUN gimme $GIMME_GO_VERSION

# Setup Go
ENV GOROOT /root/.gimme/versions/go$GIMME_GO_VERSION.linux.amd64
ENV GOPATH /go
ENV PATH $GOROOT/bin:$GOPATH/bin:$PATH
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

# Setup entrypoint
WORKDIR $GOPATH
