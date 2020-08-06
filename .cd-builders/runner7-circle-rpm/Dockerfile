FROM centos:6

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

RUN yum groupinstall -y development
RUN yum -y install \
  which perl-ExtUtils-MakeMaker \
  centos-release-scl pkgconfig \
  curl-devel expat-devel gettext-devel openssl-devel zlib-devel bzip2 \
  glibc-static tar libtool

# Git
RUN curl -OL https://www.kernel.org/pub/software/scm/git/git-2.10.1.tar.gz
# --no-same-owner: git tarball has a file with UID 110493 which makes pulling this image fail, because we use docker user namespacing and we can't have >65K UIDs.
RUN tar xzf git-2.10.1.tar.gz --no-same-owner
RUN cd git-2.10.1 && make prefix=/usr/local all
RUN cd git-2.10.1 && make prefix=/usr/local install
RUN rm -rf git-2.10.1 git-2.10.1.tar.gz

RUN git config --global user.email "gitlab@stackstate" && git config --global user.name "Gitlab Stackstate"

# RVM
COPY ./rvm/gpg-keys /gpg-keys
RUN gpg --import /gpg-keys/*
RUN rm -rf /gpg-keys
RUN curl -sSL https://get.rvm.io | bash -s stable --version latest-1.29
RUN /bin/bash -l -c "rvm requirements"
RUN /bin/bash -l -c "rvm install 2.3 && rvm cleanup all"
RUN /bin/bash -l -c "gem install bundler --no-document"

# Gimme
RUN curl -sL -o /bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme
RUN chmod +x /bin/gimme
RUN gimme $GIMME_GO_VERSION
COPY ./gobin.sh /etc/profile.d/

# Conda
RUN curl -sL -o ~/miniconda.sh https://repo.anaconda.com/miniconda/Miniconda3-${DD_CONDA_VERSION}-Linux-x86_64.sh
RUN bash ~/miniconda.sh -b
COPY ./conda.sh /etc/profile.d/
ENV PATH "${CONDA_PATH}/bin:${PATH}"

ENV PKG_CONFIG_LIBDIR $PKG_CONFIG_LIBDIR:$CONDA_PATH/lib/pkgconfig

# Setup pythons
RUN conda create -n ddpy2 python python=2
RUN conda create -n ddpy3 python python=3.8

# Autoconf
# We need a newer version of autoconf to compile procps-ng (installing 2.69 over 2.63).
RUN curl -sL -o /tmp/autoconf-2.69.tar.gz https://ftp.gnu.org/gnu/autoconf/autoconf-2.69.tar.gz \
    && cd /tmp \
    && tar -xvf /tmp/autoconf-2.69.tar.gz --no-same-owner \
    && cd autoconf-2.69 \
    && ./configure \
    && make && make install \
    && cd / \
    && rm -rf /tmp/autoconf-2.69 /tmp/autoconf-2.69.tar.gz

# IBM MQ
# IBM MQ is required in the builder.
# It cannot be shipped with the agent for licensing reasons,
# so it can't be included in Omnibus
RUN mkdir -p /opt/mqm \
    && curl "https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/${IBM_MQ_VERSION}-IBM-MQC-Redist-LinuxX64.tar.gz" -o /tmp/mq_client.tar.gz \
    && tar -C /opt/mqm -xf /tmp/mq_client.tar.gz \
    && rm -rf /tmp/mq_client.tar.gz
# CONDA prep
RUN conda init bash

# Update pip, setuptools and misc deps
RUN source /root/.bashrc && conda activate ddpy2 \
    && pip install -i https://pypi.python.org/simple pip==${DD_PIP_VERSION} \
    && pip install --ignore-installed setuptools==${DD_SETUPTOOLS_VERSION} \
    && pip install invoke distro==1.4.0 awscli==1.16.240

# Update pip, setuptools and misc deps
RUN source /root/.bashrc && conda activate ddpy3 \
    && pip install -i https://pypi.python.org/simple pip==${DD_PIP_VERSION} \
    && pip install --ignore-installed setuptools==${DD_SETUPTOOLS_VERSION} \
    && pip install invoke distro==1.4.0 awscli==1.16.240

# Add systemd headers
COPY ./rpm-headers/systemd /usr/include/systemd

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
RUN rm -r /usr/src/kernels/* && \
    curl -Sl -O https://dd-agent-omnibus.s3.amazonaws.com/kernel-4.9-headers-rpm-x64.tgz && \
    tar xf kernel-4.9-headers-rpm-x64.tgz --no-same-owner --strip 1 -C /usr && \
    rm kernel-4.9-headers-rpm-x64.tgz

# Update GCC: CentOS6 gcc-4.4 is a little too far behind what we use with the debian builder
RUN curl -o /etc/yum.repos.d/devtools-1.1.repo https://people.centos.org/tru/devtools-1.1/devtools-1.1.repo
RUN yum --enablerepo=testing-1.1-devtools-6 -y install devtoolset-1.1-gcc devtoolset-1.1-gcc-c++

ENV CC=/opt/centos/devtoolset-1.1/root/usr/bin/gcc
ENV CPP=/opt/centos/devtoolset-1.1/root/usr/bin/cpp
ENV CXX=/opt/centos/devtoolset-1.1/root/usr/bin/c++

# Download and install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $GOPATH/bin v1.21.0

# Entrypoint
COPY ./entrypoint.sh /
RUN chmod +x /entrypoint.sh

# create the agent build folder within $GOPATH
RUN mkdir -p /go/src/github.com/DataDog/datadog-agent

# Force umask to 0022
RUN echo "umask 0022" >> /root/.bashrc

ENTRYPOINT ["/entrypoint.sh"]
