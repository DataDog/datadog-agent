ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
FROM ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS baseimage
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME
LABEL baseimage.os="ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.title="Datadog Agent"

ARG CIBUILD
# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CIBUILD" = "true" ]; then \
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

FROM baseimage AS extract-base
RUN apt install --no-install-recommends -y curl ca-certificates maven xz-utils

############################################################
#  Preparation stage: retrieval of 3rd party dependencies  #
############################################################

FROM extract-base AS external-deps
ARG WITH_JMX_FIPS
ARG TARGETARCH
ARG GENERAL_ARTIFACTS_CACHE_BUCKET_URL

WORKDIR /output

# Get s6-overlay
ENV S6_VERSION="v2.2.0.3"
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${GENERAL_ARTIFACTS_CACHE_BUCKET_URL:+${GENERAL_ARTIFACTS_CACHE_BUCKET_URL}/s6-overlay}
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${JUST_CONTAINERS_DOWNLOAD_LOCATION:-https://github.com/just-containers/s6-overlay/releases/download}
RUN S6ARCH=$([ "$TARGETARCH" = "amd64" ] && echo "amd64" || echo "aarch64") && curl -L ${JUST_CONTAINERS_DOWNLOAD_LOCATION}/${S6_VERSION}/s6-overlay-${S6ARCH}.tar.gz -o /output/s6.tgz
COPY s6.$TARGETARCH.sha256 /output/s6.$TARGETARCH.sha256
# To calculate S6_SHA256SUM for a specific version, run:
# curl -L https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-<arch>.tar.gz | sha256sum
RUN echo "$(cat /output/s6.$TARGETARCH.sha256) /output/s6.tgz" | sha256sum -c - && rm -f /output/s6.$TARGETARCH.sha256

# Download BouncyCastle FIPS provider jar files.
COPY bouncycastle-fips/pom.xml /opt/bouncycastle-fips/
RUN if [ -n "$WITH_JMX_FIPS" ]; then cd /opt/bouncycastle-fips && mvn dependency:copy-dependencies; else mkdir -p /opt/bouncycastle-fips/target/dependency; fi

########################################################################
#  Construct the base image for the release, everything but the Agent  #
########################################################################

FROM baseimage AS release-base

ENV DOCKER_DD_AGENT=true \
    PATH=/opt/datadog-agent/bin/agent/:/opt/datadog-agent/embedded/bin/:$PATH \
    CURL_CA_BUNDLE=/opt/datadog-agent/embedded/ssl/certs/cacert.pem \
    # Pass envvar variables to agents
    S6_KEEP_ENV=1 \
    # Direct all agent logs to stdout
    S6_LOGGING=0 \
    # Exit container if entrypoint fails
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    # Allow readonlyrootfs
    S6_READ_ONLY_ROOT=1

# make sure we have recent dependencies -- CVE-fixing time!
RUN apt full-upgrade -y \
  # Install iproute2 package for the ss utility that is used by the network check.
  # When the network check will have switched from using ss to directly parsing /proc/net/tcp,
  # this can be removed
  # Install libseccomp2 as required by `nosys-seccomp` wrapper
  && apt install -y iproute2 libseccomp2 tzdata adduser

ARG WITH_JMX
ARG WITH_JMX_FIPS

# Install openjdk-11-jre-headless on jmx flavor
RUN if [ -n "$WITH_JMX" ]; then echo "Pulling openjdk-11 from testing" \
  && mkdir -p /usr/share/man/man1 \
  && apt install --no-install-recommends -y openjdk-11-jre-headless \
  && apt clean; fi

# Extract s6-overlay
#
# This step is dependant on the distribution's filesystem layout:
# - When Buster moved to merged-usr (/bin/ as a symlink to /usr/bin),
#   we had to change the extraction logic, see #1591
# - The debian image is now built with merged-usr explicitly disabled,
#   see https://github.com/debuerreotype/debuerreotype/pull/50
# - Ubuntu 20.10 uses the symlink /bin -> /usr/bin
COPY --from=external-deps /output/s6.tgz /
RUN tar xzf s6.tgz -C / --exclude="./bin" \
  && tar xzf s6.tgz -C /usr ./bin \
  && rm s6.tgz

# Override the exit script by ours to fix --pid=host operations
RUN  mv /etc/s6/init/init-stage3 /etc/s6/init/init-stage3-original
COPY init-stage3          /etc/s6/init/init-stage3
COPY init-stage3-host-pid /etc/s6/init/init-stage3-host-pid

# Copy BouncyCastle Java FIPS provider binaries and configuration
COPY --from=external-deps /opt/bouncycastle-fips/target/dependency/*.jar /opt/bouncycastle-fips/
COPY --chmod=644 bouncycastle-fips/java.security /opt/bouncycastle-fips/
COPY --chmod=644 bouncycastle-fips/bc-fips.policy /opt/bouncycastle-fips/
RUN if [ -z "$WITH_JMX_FIPS" ]; then rm -rf /opt/bouncycastle-fips; fi
# Configure Java to use BouncyCastle FIPS provider on JMX FIPS images.
# Double equals sign for java.security.properties istructs java to replace system defaults with the contents of the new file.
ENV JAVA_TOOL_OPTIONS="${WITH_JMX_FIPS:+--module-path=/opt/bouncycastle-fips -Djava.security.properties==/opt/bouncycastle-fips/java.security -Dpolicy.url.2=file:/opt/bouncycastle-fips/bc-fips.policy}"

# S6 entrypoint, service definitions, healthcheck probe
COPY s6-services /etc/services.d/
COPY cont-init.d /etc/cont-init.d/
COPY probe.sh initlog.sh secrets-helper/readsecret.py secrets-helper/readsecret.sh secrets-helper/readsecret_multiple_providers.sh /

##########################################################
#  Preparation stage: extract Agent package and cleanup  #
##########################################################

FROM extract-base AS extract
ARG TARGETARCH
ARG WITH_JMX

ARG DD_AGENT_ARTIFACT=datadog-agent*-$TARGETARCH.tar.xz
# copy everything - globbing with args wont work
COPY ${DD_AGENT_ARTIFACT} /
WORKDIR /output

# Configuration:
#   - copy default config files
COPY datadog*.yaml etc/datadog-agent/

# Installation information
COPY install_info etc/datadog-agent/

# Extract and cleanup:
#   - unused systemd unit
#   - GPL sources for embedded software  # FIXME: move upstream
#   - docs and manpages                  # FIXME: move upstream
#   - static libraries                   # FIXME: move upstream
#   - jmxfetch on nojmx build

RUN --mount=from=artifacts,target=/artifacts \
 find /artifacts -maxdepth 1 -type f -name "datadog-*-$TARGETARCH.tar.xz" ! -name "$DD_AGENT_ARTIFACT" -exec rm {} \; \
 && find /artifacts -maxdepth 1 -name "${DD_AGENT_ARTIFACT}" -exec tar xvf {} -C . \; \
 && rm -rf usr etc/init lib \
    go/ \
    opt/datadog-agent/sources \
    opt/datadog-agent/embedded/share/doc \
    opt/datadog-agent/embedded/share/man \
    # self-test certificates that are detected (false positive) as private keys
    opt/datadog-agent/embedded/lib/python*/site-packages/future/backports/test \
 && find opt/datadog-agent/ -iname "*.a" -delete \
 && if [ -z "$WITH_JMX" ]; then rm -rf opt/datadog-agent/bin/agent/dist/jmx; fi \
 && mkdir conf.d checks.d

###########################################################
#  Release docker image construction including the Agent  #
###########################################################

FROM release-base AS release
LABEL maintainer="Datadog <package@datadoghq.com>"

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA
ENV \
    # Allow User Group to exec the secret backend script.
    DD_SECRET_BACKEND_COMMAND_ALLOW_GROUP_EXEC_PERM="true" \
    # Source code integration
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

# Copy agent from extract stage
COPY --from=extract /output/ /

# Prepare for running without root
# - Create a dd-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start
RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
  && addgroup --system secret-manager \
  && usermod -a -G secret-manager dd-agent \
  && rm /var/run && mkdir -p /var/run/s6 && mkdir -p /var/run/datadog \
  && chown -R dd-agent:root /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/ \
  && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ /var/run/datadog/ \
  && chmod 755 /probe.sh /initlog.sh \
  && chown root:secret-manager /readsecret.py /readsecret.sh /readsecret_multiple_providers.sh \
  && chmod 550 /readsecret.py /readsecret.sh /readsecret_multiple_providers.sh

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

# cleaning up
# We remove /etc/ssl because in JMX images, openjdk triggers installation of ca-certificates-java
# which writes to /etc/ssl. Yet, in the Agent we only rely on certs shipped in the Agent package.
RUN rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /etc/ssl
RUN ln -sf /opt/datadog-agent/embedded/ssl /etc/ssl

# Enable FIPS if needed
RUN if [ -x /opt/datadog-agent/embedded/bin/fipsinstall.sh ]; then \
  /opt/datadog-agent/embedded/bin/fipsinstall.sh; \
fi

# Update if optional OTel Agent process should not run
RUN if [ ! -f /opt/datadog-agent/embedded/bin/otel-agent ]; then \
  rm -rf /etc/services.d/otel ; \
  else \
  mv /etc/datadog-agent/otel-config.yaml.example /etc/datadog-agent/otel-config.yaml; fi

RUN find /etc -type d,f -perm -o+w -print0 | xargs -r -0 chmod g-w,o-w

# Add Debian snapshot date for debugging
RUN date +%Y%m%dT000000Z > .debian_repo_snapshot_date

# Expose DogStatsD and trace-agent ports
EXPOSE 8125/udp 8126/tcp

HEALTHCHECK --interval=30s --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

# Leave following directories RW to allow use of kubernetes readonlyrootfs flag
VOLUME ["/var/run/s6", "/var/log/datadog"]

# Ensure the glibc doesn't try to call syscalls that may not be supported
COPY --from=nosys-seccomp /tmp/nosys.so /opt/lib/nosys.so
ENV LD_PRELOAD=/opt/lib/nosys.so

# Single entrypoint
COPY --chmod=755 entrypoint.sh /bin/entrypoint.sh
COPY --chmod=755 entrypoint.d /opt/entrypoints

CMD ["/bin/entrypoint.sh"]

FROM release AS test
COPY --chmod=755 test_image_contents.py /tmp/test_image_contents.py
RUN ./tmp/test_image_contents.py && rm -f ./tmp/test_image_contents.py
