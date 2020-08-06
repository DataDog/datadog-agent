#FROM debian:wheezy-backports
FROM golang:1.13.11

RUN sed -i 's/^#\s*\(deb.*universe\)$/\1/g' /etc/apt/sources.list \
    && sed -i 's/^#\s*\(deb.*multiverse\)$/\1/g' /etc/apt/sources.list \
    && sed -i 's/main/main contrib non-free/' /etc/apt/sources.list

# Build Args
ARG GIMME_GO_VERSION=1.13.11
ARG DD_CONDA_VERSION=4.7.10
ARG DD_PIP_VERSION=19.1
ARG DD_SETUPTOOLS_VERSION=41.0.1
ARG IBM_MQ_VERSION=9.1.5.0
ARG CMAKE_VERSION=3.14.4
ARG CLANG_VERSION=8.0.0

# Environment
ENV GOPATH /go
ENV GIMME_GO_VERSION $GIMME_GO_VERSION
ENV DD_PIP_VERSION $DD_PIP_VERSION
ENV DD_SETUPTOOLS_VERSION $DD_SETUPTOOLS_VERSION
ENV IBM_MQ_VERSION $IBM_MQ_VERSION
ENV CMAKE_VERSION $CMAKE_VERSION
ENV CLANG_VERSION $CLANG_VERSION
ENV CONDA_PATH /root/miniconda3
ENV DD_CONDA_VERSION $DD_CONDA_VERSION

# Mitigation for CVE-2019-3462
#[VS] RUN echo 'Acquire::http::AllowRedirect"false";' >> /etc/apt/apt.conf.d/20datadog
# Ignore expired repos signature
# Wheezy is EOL, security updates repo will not get any newer updates, or will do so
# in arbitrary, unscheduled timeframes. At the time of this writing the repo has
# expired making the following option necessary for apt to work.
#RUN echo 'Acquire::Check-Valid-Until "false";' >> /etc/apt/apt.conf.d/20datadog

#RUN echo "deb http://archive.debian.org/debian wheezy main contrib non-free" > /etc/apt/sources.list && \
# echo "deb http://archive.debian.org/debian wheezy-backports main contrib non-free" > /etc/apt/sources.list.d/backports.list && \
# echo "deb http://archive.debian.org/debian-security wheezy/updates main contrib non-free" > /etc/apt/sources.list.d/security.list


# [VS]
# RUN mkdir -p /etc/apt/source.list.d/ && \
#     echo "deb http://deb.debian.org/debian/ stretch main contrib non-free" > /etc/apt/source.list.d/sts_extra.list && \
#     echo "deb-src http://deb.debian.org/debian stretch main contrib non-free" >> /etc/apt/source.list.d/sts_extra.list

RUN echo 'Acquire::Check-Valid-Until "false";' >> /etc/apt/apt.conf.d/20stackstate

# [/VS]

RUN apt-get update && apt-get install -y fakeroot curl git procps bzip2 \
  build-essential pkg-config\
  rpm tar gettext libtool autopoint autoconf pkg-config flex \
  selinux-basics


#RUN apt-get install -y libsystemd-journal-dev/wheezy-backports
RUN apt-get install -y libsystemd-dev

# The license does not recognize debian based systems for some reason
# It's a bug in this version. So, it has been rewritten
#COPY ./mqlicense.sh /mqlicense.sh

# IBM MQ
# IBM MQ is required in the builder.
RUN mkdir -p /opt/mqm \
    && curl "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/${IBM_MQ_VERSION}-IBM-MQC-Redist-LinuxX64.tar.gz" -o /tmp/mq_client.tar.gz \
    && tar -C /opt/mqm -xf /tmp/mq_client.tar.gz \
    && rm -rf /tmp/mq_client.tar.gz

# Conda
RUN curl -sL -o ~/miniconda.sh https://repo.anaconda.com/miniconda/Miniconda3-${DD_CONDA_VERSION}-Linux-x86_64.sh
RUN bash ~/miniconda.sh -b
COPY ./conda.sh /etc/profile.d/

# RVM
# [VS]
COPY ./rvm/gpg-keys /gpg-keys
RUN gpg --import /gpg-keys/*
RUN rm -rf /gpg-keys
RUN curl -sSL https://get.rvm.io | bash -s stable --version latest-1.29

# # Ruby,,,
RUN mkdir -p /usr/local/etc \
    && { \
    echo 'install: --no-document'; \
    echo 'update: --no-document'; \
    } >> /usr/local/etc/gemrc

# ENV BUNDLER_VERSION 1.15.3

# RUN gem install bundler --version "$BUNDLER_VERSION"

RUN /bin/bash -l -c "rvm requirements"
RUN /bin/bash -l -c "rvm install 2.4.2 && rvm cleanup all"
RUN /bin/bash -l -c "gem install bundler --no-document"

# CONDA prep
ENV PKG_CONFIG_LIBDIR "${PKG_CONFIG_LIBDIR}:${CONDA_PATH}/lib/pkgconfig"
ENV PATH "${CONDA_PATH}/bin:${PATH}"
RUN conda init bash

# Setup pythons
RUN conda create -n ddpy2 python python=2
RUN conda create -n ddpy3 python python=3.8

# Update pip, setuptools and misc deps
RUN /bin/bash -c "source /root/.bashrc && conda activate ddpy2 \
    && pip install -i https://pypi.python.org/simple pip==${DD_PIP_VERSION} \
    && pip install --ignore-installed setuptools==${DD_SETUPTOOLS_VERSION} \
    && pip install invoke distro==1.4.0 awscli==1.16.240"

# Update pip, setuptools and misc deps
RUN /bin/bash -c  "source /root/.bashrc && conda activate ddpy3 \
    && pip install -i https://pypi.python.org/simple pip==${DD_PIP_VERSION} \
    && pip install --ignore-installed setuptools==${DD_SETUPTOOLS_VERSION} \
    && pip install invoke distro==1.4.0 awscli==1.16.240"

# [VS] not needed if we base on GO base image.
# Gimme
#RUN curl -sL -o /bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme
#RUN chmod +x /bin/gimme
#RUN gimme $GIMME_GO_VERSION
COPY ./gobin.sh /etc/profile.d/

# Docker
# Pin docker to before they broke wheezy
#[VS] RUN curl -fsSL https://raw.githubusercontent.com/docker/docker-install/a34555fc0214be705330911071a8c3357f26e40b/install.sh | sed -e 's/ftp\.debian\.org/archive.debian.org/g' | sh

# CMake
RUN set -ex \
    && curl -sL -o cmake.sh https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}-Linux-x86_64.sh \
    && mkdir -p /opt/cmake/ \
    && sh cmake.sh --skip-license --prefix=/opt/cmake \
    && ln -s /opt/cmake/bin/cmake /usr/local/bin/cmake \
    && rm cmake.sh

# Install clang and llvm version 8
# Using build for sles11 because the versions built for other distros target glibcs that are too new to be used from this image
RUN curl -LO https://releases.llvm.org/${CLANG_VERSION}/clang+llvm-${CLANG_VERSION}-x86_64-linux-sles11.3.tar.xz && \
    tar -xf clang+llvm-${CLANG_VERSION}-x86_64-linux-sles11.3.tar.xz --no-same-owner --strip 1 -kC /usr/ && \
    rm clang+llvm-${CLANG_VERSION}-x86_64-linux-sles11.3.tar.xz

# To build the EBPF code we need kernel headers for Linux 4.9
RUN curl -Sl -O https://dd-agent-omnibus.s3.amazonaws.com/kernel-4.9-headers-deb-x64.tgz && \
    tar xf kernel-4.9-headers-deb-x64.tgz --no-same-owner --strip 1 -C /usr && \
    rm kernel-4.9-headers-deb-x64.tgz

# Download and install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $GOPATH/bin v1.21.0

# Entrypoint
COPY ./entrypoint.sh /
RUN chmod +x /entrypoint.sh

# create the agent build folder within $GOPATH
RUN mkdir -p /go/src/github.com/StackVista/stackstate-agent

# Force umask to 0022
RUN echo "umask 0022" >> /root/.bashrc

# STS

RUN git config --global user.email "gitlab@stackstate" && git config --global user.name "Gitlab Stackstate"

# For deb package building
RUN apt-get install -y apt-utils libtool && apt-get install -y --only-upgrade libtool

# For some tests
RUN apt-get install sudo

# TODO: to be moved to conda virtualenvs
#RUN pip install awscli boto3 sa-rpm-s3
ADD rpmmacros /root/.rpmmacros
# yum-utils #https://pkgs.org/download/yum-utils
#RUN apt-get install -y libkrb5-dev unixodbc-dev rpm python-deltarpm createrepo bison yum-utils

# /STS

#[VS]
RUN go version

ENTRYPOINT ["/entrypoint.sh"]
