FROM golang:1.25.6-bookworm@sha256:fae400019c7fb0400bca0b749fde2015f993ff0e0293ca6f8dbaf919650bb419

RUN apt-get update -y && apt-get dist-upgrade -y && \
    apt-get install -y sudo && \
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

VOLUME /go
