ARG BASE_IMAGE_UBUNTU_VERSION=26.04
ARG BASE_IMAGE_REGISTRY=docker.io/library

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION

ENV DEBIAN_FRONTEND=noninteractive

ARG CI
# APT mirror resilience in CI (see Dockerfiles/agent/Dockerfile for the rationale):
# handle both the deb822 ubuntu.sources (multi-URI failover) and the legacy
# one-line sources.list (mirror+file), using Canonical's global endpoint first
# with Leaseweb as a fallback, with a short Acquire timeout + Retries 1.
RUN if [ "$CI" = "true" ]; then \
  printf 'Acquire::Retries "1";\nAcquire::http::Timeout "10";\nAcquire::https::Timeout "10";\n' > /etc/apt/apt.conf.d/99datadog-mirror-resilience && \
  printf 'http://archive.ubuntu.com/ubuntu\nhttp://mirror.leaseweb.net/ubuntu\n' > /etc/apt/mirrorlist.main && \
  printf 'http://ports.ubuntu.com/ubuntu-ports\nhttp://mirror.leaseweb.net/ubuntu-ports\n' > /etc/apt/mirrorlist.ports && \
  if [ -f /etc/apt/sources.list ]; then \
    sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
           -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
           -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' /etc/apt/sources.list; \
  fi && \
  if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then \
    sed -i -E \
      -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*archive\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
      -e 's#^(URIs:[[:space:]]+)https?://security\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
      -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*ports\.ubuntu\.com/ubuntu-ports/?[[:space:]]*$#\1http://ports.ubuntu.com/ubuntu-ports http://mirror.leaseweb.net/ubuntu-ports#' \
      /etc/apt/sources.list.d/ubuntu.sources; \
  fi; \
  fi

RUN set -eu && \
  apt update; \
  apt install -y \
  --no-install-recommends \
  ca-certificates \
  curl \
  systemd \
  systemd-cron \
  sudo \
  openssh-server;

# setup ssh
RUN mkdir -p /etc/ssh/sshd_config.d && echo "PermitRootLogin without-password" > /etc/ssh/sshd_config.d/allow_root.conf
ARG DOCKER_HOST_SSH_PUBLIC_KEY
RUN mkdir -p /root/.ssh && echo $DOCKER_HOST_SSH_PUBLIC_KEY >> /root/.ssh/authorized_keys
RUN chmod 600 /root/.ssh/authorized_keys

RUN service ssh start

EXPOSE 22

CMD ["/usr/sbin/init"]
