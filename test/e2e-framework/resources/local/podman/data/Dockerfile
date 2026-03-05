ARG BASE_IMAGE_UBUNTU_VERSION=20.04
ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION

ENV DEBIAN_FRONTEND=noninteractive

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
