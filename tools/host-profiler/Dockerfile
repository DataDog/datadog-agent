FROM golang:1.25.5-bookworm@sha256:5117d68695f57faa6c2b3a49a6f3187ec1f66c75d5b080e4360bfe4c1ada398c

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

USER build

WORKDIR /app

VOLUME /go
