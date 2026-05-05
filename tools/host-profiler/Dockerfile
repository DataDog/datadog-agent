FROM debian:trixie-slim AS dda
ARG DDA_VERSION=v0.29.0
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then DDA_ARCH="x86_64"; \
    elif [ "$ARCH" = "arm64" ]; then DDA_ARCH="aarch64"; fi && \
    curl -fsSL --retry 4 "https://github.com/DataDog/datadog-agent-dev/releases/download/${DDA_VERSION}/dda-${DDA_ARCH}-unknown-linux-gnu.tar.gz" \
    | tar -xzf - -C /usr/local/bin && \
    chmod +x /usr/local/bin/dda

FROM golang:1.25.9-trixie

RUN cross_debian_arch=$(uname -m | sed -e 's/aarch64/amd64/'  -e 's/x86_64/arm64/'); \
    cross_pkg_arch=$(uname -m | sed -e 's/aarch64/x86-64/' -e 's/x86_64/aarch64/'); \
    dpkg --add-architecture amd64 && dpkg --add-architecture arm64 && \
    apt-get update -y && apt-get dist-upgrade -y && \
    apt-get install -y sudo less curl wget make git cmake unzip \
        libc6-dev g++ gcc pkgconf clang-17 clang-format-17 \
        gcc-${cross_pkg_arch}-linux-gnu libc6-${cross_debian_arch}-cross \
        musl-dev:amd64 musl-dev:arm64 && \
    apt-get clean autoclean && \
    apt-get autoremove --yes

COPY --from=dda /usr/local/bin/dda /usr/local/bin/dda
ENV DDA_INTERACTIVE=false

ARG UID
ARG GID

RUN addgroup --gid ${GID} build
RUN useradd -ms /bin/bash build -u ${UID} -g ${GID}
RUN echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
RUN adduser build sudo
RUN chown -R build:build /go
ENV GOTOOLCHAIN=auto

USER build

RUN dda self dep sync -f legacy-tasks

WORKDIR /app

# Add host-profiler tools to PATH for convenience
ENV PATH="/app/tools/host-profiler:${PATH}"

VOLUME /go

CMD ["build_and_launch.sh"]
