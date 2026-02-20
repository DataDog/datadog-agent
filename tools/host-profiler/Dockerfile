FROM golang:1.25.6-bookworm@sha256:fae400019c7fb0400bca0b749fde2015f993ff0e0293ca6f8dbaf919650bb419

RUN cross_debian_arch=$(uname -m | sed -e 's/aarch64/amd64/'  -e 's/x86_64/arm64/'); \
    cross_pkg_arch=$(uname -m | sed -e 's/aarch64/x86-64/' -e 's/x86_64/aarch64/'); \
    dpkg --add-architecture amd64 && dpkg --add-architecture arm64 && \
    apt-get update -y && apt-get dist-upgrade -y && \
    apt-get install -y sudo less lsb-release software-properties-common \
        curl wget make git cmake unzip libc6-dev g++ gcc pkgconf \
        gcc-${cross_pkg_arch}-linux-gnu libc6-${cross_debian_arch}-cross \
        musl-dev:amd64 musl-dev:arm64 && \
    curl -sSf https://apt.llvm.org/llvm.sh | bash -s -- 17 && \
    apt-get install -y clang-format-17 && \
    apt-get clean autoclean && \
    apt-get autoremove --yes

ARG UID
ARG GID

RUN addgroup --gid ${GID} build
RUN useradd -ms /bin/bash build -u ${UID} -g ${GID}
RUN echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
RUN adduser build sudo
RUN chown -R build:build /go
ENV GOTOOLCHAIN=auto

USER build

WORKDIR /app

# Add host-profiler tools to PATH for convenience
ENV PATH="/app/tools/host-profiler:${PATH}"

VOLUME /go

CMD ["build_and_launch.sh"]
