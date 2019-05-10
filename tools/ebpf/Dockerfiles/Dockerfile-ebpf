# Used to compile eBPF code
FROM golang:1.11.5

ENV GOPATH /go

RUN apt-get update && \
        apt-get install -y linux-headers-4.9.0-9-all git python3-pip

# Required to run the invoke tasks
RUN pip3 install invoke==1.0 pyyaml==5.1

# Install clang and llvm version 8
# 9 does not seem to work correctly
RUN wget -O - https://apt.llvm.org/llvm-snapshot.gpg.key | apt-key add - && \
        echo "deb http://apt.llvm.org/stretch/ llvm-toolchain-stretch-8 main \
deb-src http://apt.llvm.org/stretch/ llvm-toolchain-stretch-8 main" >> /etc/apt/sources.list && \
        apt-get update && \
        apt-get install -y clang-8 llvm-8 && \
        ln -sf /usr/bin/clang-8 /usr/bin/clang && \
        ln -sf /usr/bin/llc-8 /usr/bin/llc

RUN mkdir -p /src /go
