ARG BASE_IMAGE_UBUNTU_VERSION=26.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS baseimage
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME
LABEL baseimage.os="ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL maintainer="Datadog <package@datadoghq.com>"

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
# Canonical's global archive.ubuntu.com / ports.ubuntu.com endpoints are used first,
# with mirror.leaseweb.net as an ordered fallback. A short Acquire timeout + Retries 1
# turns a slow mirror into a fast error that triggers failover. Related:
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

ENV DEBIAN_FRONTEND=noninteractive

FROM baseimage AS nosys-seccomp
RUN apt update && apt install --no-install-recommends -y gcc libc6-dev libseccomp-dev
COPY nosys-seccomp/nosys.c   /tmp/nosys.c
COPY nosys-seccomp/nosys.sym /tmp/nosys.sym
RUN gcc -pipe -Wall -Wextra -O2 -shared -fPIC -Wl,--version-script=/tmp/nosys.sym -o /tmp/nosys.so /tmp/nosys.c  -lseccomp

# ------------------------------
# Final stage
# ------------------------------

FROM baseimage AS release

LABEL maintainer="Datadog <package@datadoghq.com>"

ENV DEBIAN_FRONTEND=noninteractive
# make sure we have recent dependencies -- CVE-fixing time!
RUN apt update && \
    apt full-upgrade -y && \
    apt install -y tzdata ca-certificates adduser && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

# Prepare for running without root
# - Create a dd-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start
RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
  && rm /var/run && mkdir -p /var/run/datadog /var/log/datadog/ /etc/datadog-agent \
  && chown -R dd-agent:root /etc/datadog-agent/ /var/log/datadog/ /var/run/datadog/ \
  && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /var/log/datadog/ /var/run/datadog/

# Check that the UID of dd-agent is still 100.
#
# The exact numeric value of the UID of the dd-agent user shouldn’t matter.
# But people that don’t want to let the agent run as root might want to explicitly set a non-root user in their k8s security context.
# And, in a k8s security context, we can only specify a numeric UID and not a username.
# So, if the UID of the dd-agent user happen to change again and cannot be forced to 100 (because of a conflict), we need to update
# * the documentation https://docs.datadoghq.com/data_security/kubernetes/#running-container-as-root-user
#   (see PR https://github.com/DataDog/documentation/pull/21889)
# * https://datadoghq.atlassian.net/wiki/spaces/TS/pages/2615709591/Why+the+containerized+Agent+runs+as+root#Agent-user
RUN [ "$(getent passwd dd-agent | cut -d: -f 3)" -eq 100 ]

# Configuration:
#   - copy default config files
COPY datadog*.yaml etc/datadog-agent/

# Installation information
COPY install_info etc/datadog-agent/

# Ensure the glibc doesn't try to call syscalls that may not be supported
COPY --from=nosys-seccomp /tmp/nosys.so /opt/lib/nosys.so
ENV LD_PRELOAD=/opt/lib/nosys.so \
    PATH="/opt/datadog-agent/embedded/bin:${PATH}"

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA
ENV DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA} \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL}
