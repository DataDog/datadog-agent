FROM ubuntu:18.04

ENV CMAKE_NAME cmake-3.13.3-Linux-x86_64
ENV CMAKE_ARCHIVE $CMAKE_NAME.tar.gz
ENV CMAKE_DEST_DIR /cmake
ENV PATH $CMAKE_DEST_DIR/bin/:$PATH

# Pre-requisites
RUN set -ex \
    && apt-get update && apt-get install -y --no-install-recommends \
        gnupg ca-certificates \
        gcc g++ make git ssh \
        wget \
        python-dev python3.7-dev python3-distutils \
        golang

# Project dependencies
RUN set -ex \
    # clang-format v8
    && echo "deb http://apt.llvm.org/bionic/ llvm-toolchain-bionic-8 main" >> /etc/apt/sources.list \
    && wget -O - https://apt.llvm.org/llvm-snapshot.gpg.key | apt-key add - \
    && apt-get update && apt-get -t llvm-toolchain-bionic-8 install -y --no-install-recommends \
        clang-format-8 \
    # cmake 3.13
    && wget https://github.com/Kitware/CMake/releases/download/v3.13.3/$CMAKE_ARCHIVE \
    && tar xzf $CMAKE_ARCHIVE \
    && mv $CMAKE_NAME $CMAKE_DEST_DIR \
    && rm $CMAKE_ARCHIVE
