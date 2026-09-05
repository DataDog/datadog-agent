ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS nosys-seccomp
ARG CI
ENV DEBIAN_FRONTEND=noninteractive
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
RUN apt update && apt install --no-install-recommends -y gcc libc6-dev libseccomp-dev
COPY nosys-seccomp/nosys.c   /tmp/nosys.c
COPY nosys-seccomp/nosys.sym /tmp/nosys.sym
RUN gcc -pipe -Wall -Wextra -O2 -shared -fPIC -Wl,--version-script=/tmp/nosys.sym -o /tmp/nosys.so /tmp/nosys.c -lseccomp

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS release-base
ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL baseimage.os "ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name "ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name "ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.title "Datadog Cluster Agent"
LABEL org.opencontainers.image.authors="Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.source="https://github.com/DataDog/datadog-agent"
LABEL org.opencontainers.image.vendor="Datadog, Inc."

ARG CI
# APT mirror resilience in CI:
# Ubuntu EC2 AMIs point apt at a single regional mirror (us-east-1) with no real
# failover. When that mirror is slow-but-reachable, apt would stall until the CI
# job timeout: the `mirror` method only falls back on a fully-unreachable mirror,
# and Acquire::Retries retries the SAME URI. The base image version is
# configurable (BASE_IMAGE_UBUNTU_VERSION), so we handle both source formats:
#   - deb822 ubuntu.sources (>= 24.04): rewrite the URIs: fields to an ordered
#     multi-URI list so apt fails over to the next mirror on error.
#   - legacy one-line sources.list (< 24.04): the format can't list multiple URIs
#     per entry, so use mirror+file with a peer mirrorlist (fallback on an
#     unreachable mirror).
# In both cases Canonical's global archive/ports.ubuntu.com endpoint is used first
# with mirror.leaseweb.net as a fallback, and a short Acquire
# timeout + Retries 1 turns a slow mirror into a fast error. Related:
# test/new-e2e/tests/installer/host/host.go (ConfigureAptMirrors).
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

ENV PATH="/opt/datadog-agent/bin/:$PATH" \
    DOCKER_DD_AGENT="true"

# Install key tools
#  Some are needed for Agent install and operation, others are for TSE support.  Those have to be
#  available in the image so we can mount it readonly.  We install them both categories at the
#  same time for speed.
# For agent install and operation: ca-certificates, tzdata, adduser
# For TSE operations: curl
RUN apt-get update \
    && apt full-upgrade -y \
    && apt-get install --no-install-recommends -y ca-certificates curl libseccomp2 tzdata adduser\
    && rm -rf \
      /var/cache/debconf/*-old \
      /var/lib/apt/lists/* \
      /var/log/* \
      /tmp/* \
      /var/tmp/*

COPY ./conf.d /etc/datadog-agent/conf.d
COPY ./datadog-cluster.yaml /etc/datadog-agent/datadog-cluster.yaml
COPY ./security-agent-policies/compliance/containers/cis-kubernetes*.yaml /etc/datadog-agent/compliance.d/
COPY ./security-agent-policies/compliance/containers/*.rego /etc/datadog-agent/compliance.d/
COPY ./install_info etc/datadog-agent/install_info
COPY ./private-action-runner /etc/datadog-agent/private-action-runner
COPY entrypoint.sh .
COPY readsecret.sh readsecret_multiple_providers.sh ./

####################################
# Actual docker image construction #
####################################

FROM release-base

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA
ENV \
    # Allow User Group to exec the secret backend script.
    DD_SECRET_BACKEND_COMMAND_ALLOW_GROUP_EXEC_PERM="true" \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

COPY --from=artifacts datadog-cluster-agent /opt/datadog-agent/bin/datadog-cluster-agent
# CWS Instrumentation
COPY --from=artifacts --chmod=0755 ./cws-instrumentation/ /opt/datadog-agent/bin/datadog-cws-instrumentation/
# secret-generic-connector for secret_backend_type / secret_backend_command
COPY --from=artifacts secret-generic-connector /opt/datadog-agent/bin/secret-generic-connector

RUN addgroup --system secret-manager \
    && chmod 755 entrypoint.sh \
    && chown root:secret-manager readsecret.sh readsecret_multiple_providers.sh /opt/datadog-agent/bin/secret-generic-connector \
    && chmod 550 readsecret.sh readsecret_multiple_providers.sh /opt/datadog-agent/bin/secret-generic-connector \
    && chmod g+r,g+w,g+X -R /etc/datadog-agent/ \
    && chmod +x /opt/datadog-agent/bin/datadog-cluster-agent \
    && ln -s /opt/datadog-agent/bin/datadog-cluster-agent /opt/datadog-agent/bin/agent

# Allow running as an unprivileged user:
# - General case is the dd-agent user
# - OpenShift uses a random UID in the root group
#
# Containerd does not preserve permissions when mounting a volume on top
# of an empty folder. Creating .placeholder files as a workaround.
#
RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
    && addgroup --system secret-manager \
    && usermod -a -G secret-manager dd-agent \
    && mkdir -p /var/log/datadog/ /conf.d /opt/datadog-agent/run \
    && touch /var/log/datadog/.placeholder \
    && touch /tmp/.placeholder \
    && chown -R dd-agent:root /etc/datadog-agent/ /var/log/datadog/ /conf.d /tmp/ /opt/datadog-agent/run \
    && chmod g+r,g+w,g+X -R /var/log/datadog/ /conf.d /tmp/ /opt/datadog-agent/run \
    && chmod g+r,g-w,g+X,o-w -R /etc/datadog-agent/ \
    # Keep the PAR config read-only, but group-readable so init-volume can copy
    # it on OpenShift where containers run with a random UID in the root group.
    && chmod u=r,g=r,o= /etc/datadog-agent/private-action-runner/*

# Ensure the glibc doesn't try to call syscalls that may not be supported
COPY --from=nosys-seccomp /tmp/nosys.so /lib/x86_64-linux-gnu/nosys.so
ENV LD_PRELOAD=/lib/x86_64-linux-gnu/nosys.so

# Incompatible with the custom metrics API on port 443
# Set DD_EXTERNAL_METRICS_PROVIDER_PORT to a higher value to run as non-root
# USER dd-agent

# Leave following directories RW to allow use of readonly rootfs
VOLUME ["/etc/datadog-agent", "/var/log/datadog", "/tmp"]

ENTRYPOINT ["/entrypoint.sh"]

# No docker healthcheck, use a HTTP check
# on port 5005 and/or 443 on Kubernetes

CMD ["datadog-cluster-agent", "start"]
