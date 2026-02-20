ARG BASE_IMAGE_UBUNTU_VERSION=24.04
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
# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CI" = "true" ]; then \
  echo "http://us-east-1.ec2.archive.ubuntu.com/ubuntu\tpriority:1\nhttp://archive.ubuntu.com/ubuntu" > /etc/apt/mirrorlist.main && \
  echo "http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports" > /etc/apt/mirrorlist.ports && \
  sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
         -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
         -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' /etc/apt/sources.list; \
  fi

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update

FROM baseimage AS nosys-seccomp
RUN apt install --no-install-recommends -y gcc libc6-dev libseccomp-dev
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
RUN apt full-upgrade -y && \
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
