ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_REGISTRY=docker.io/library

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION

ENV DEBIAN_FRONTEND=noninteractive

ARG CI
# APT mirror resilience in CI (see Dockerfiles/agent/Dockerfile for the rationale):
# rewrite the deb822 URIs: fields to an ordered multi-URI list so apt fails over
# to the next mirror on error, with a short timeout + Retries 1 so a slow mirror
# becomes a fast error. us-east-1 is dropped as the chronically-slow mirror.
RUN if [ "$CI" = "true" ]; then \
  printf 'Acquire::Retries "1";\nAcquire::http::Timeout "10";\nAcquire::https::Timeout "10";\n' > /etc/apt/apt.conf.d/99datadog-mirror-resilience && \
  for f in /etc/apt/sources.list.d/ubuntu.sources; do \
    if [ -f "$f" ]; then \
      sed -i -E \
        -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*archive\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://us-west-2.ec2.archive.ubuntu.com/ubuntu http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
        -e 's#^(URIs:[[:space:]]+)https?://security\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://us-west-2.ec2.archive.ubuntu.com/ubuntu http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
        -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*ports\.ubuntu\.com/ubuntu-ports/?[[:space:]]*$#\1http://us-west-2.ec2.ports.ubuntu.com/ubuntu-ports http://ports.ubuntu.com/ubuntu-ports#' \
        "$f"; \
    fi; \
  done; \
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
