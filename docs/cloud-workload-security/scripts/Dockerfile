FROM golang:1.17-buster

ENV DEBIAN_FRONTEND noninteractive
ENV GOPATH /go

RUN apt-get update && apt-get install -y --no-install-recommends apt-utils && apt-get dist-upgrade -y && apt-get install -y --no-install-recommends \
        cmake \
        curl \
        g++ \
        gcc \
        git \
        make \
        python \
        python3-setuptools \
        python3-pip \
        wget

ENV PATH "${GOPATH}/bin:${PATH}"

# create the agent build folder within $GOPATH
WORKDIR $GOPATH/src/github.com/DataDog/datadog-agent

COPY requirements.txt requirements.txt
COPY docs/cloud-workload-security/scripts/requirements-docs.txt requirements-docs.txt
RUN python3 -m pip install wheel
RUN python3 -m pip install -r requirements.txt
RUN python3 -m pip install -r requirements-docs.txt
