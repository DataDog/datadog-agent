FROM golang:1.25-bookworm

# Install required packages
RUN apt-get update -y && apt-get dist-upgrade -y && \
    apt-get install -y sudo && \
    apt-get clean autoclean && \
    apt-get autoremove --yes

# Create build user with matching UID/GID
ARG UID=1000
ARG GID=1000

RUN groupadd -g ${GID} build || groupmod -n build $(getent group ${GID} | cut -d: -f1) && \
    useradd -ms /bin/bash build -u ${UID} -g ${GID} || usermod -l build -d /home/build -m $(getent passwd ${UID} | cut -d: -f1) && \
    echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers && \
    usermod -aG sudo build && \
    chown -R build:build /go

USER build

WORKDIR /app

VOLUME /go
