FROM ubuntu:18.04

# Pre-requisites
RUN set -ex \
    && apt-get update && apt-get install -y --no-install-recommends \
        gnupg ca-certificates \
        gcc g++ make git ssh curl pkg-config file \
        python-dev python3-setuptools \
        python3.7-dev python3-distutils python3-pip python3-yaml \
        libssl-dev libsnmp-base libsnmp-dev libpq-dev snmp-mibs-downloader libsystemd-dev \
        doxygen graphviz

# Golang
ENV GIMME_GO_VERSION 1.13.11
ENV GOROOT /root/.gimme/versions/go$GIMME_GO_VERSION.linux.amd64
ENV GOPATH /go
ENV PATH $GOROOT/bin:$GOPATH/bin:$PATH
RUN curl -sL -o /bin/gimme https://raw.githubusercontent.com/travis-ci/gimme/master/gimme
RUN chmod +x /bin/gimme
RUN gimme $GIMME_GO_VERSION
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

# Download and install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $GOPATH/bin v1.21.0

# CMake
ENV CMAKE_NAME cmake-3.13.3-Linux-x86_64
ENV CMAKE_ARCHIVE $CMAKE_NAME.tar.gz
ENV CMAKE_DEST_DIR /cmake
ENV PATH $CMAKE_DEST_DIR/bin/:$PATH
RUN set -ex \
    && curl -sL -O https://github.com/Kitware/CMake/releases/download/v3.13.3/$CMAKE_ARCHIVE \
    && tar xzf $CMAKE_ARCHIVE \
    && mv $CMAKE_NAME $CMAKE_DEST_DIR \
    && rm $CMAKE_ARCHIVE

# Other dependencies
RUN set -ex \
    # clang-format v8
    && echo "deb http://apt.llvm.org/bionic/ llvm-toolchain-bionic-8 main" >> /etc/apt/sources.list \
    && curl -sL https://apt.llvm.org/llvm-snapshot.gpg.key | apt-key add - \
    && apt-get update && apt-get -t llvm-toolchain-bionic-8 install -y --no-install-recommends \
        clang-format-8

# Setup entrypoint
WORKDIR $GOPATH
