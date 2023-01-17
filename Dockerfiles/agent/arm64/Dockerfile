FROM ubuntu:22.04 AS baseimage
LABEL baseimage.os "ubuntu jammy LTS"
LABEL baseimage.name "ubuntu:22.04"

ARG CIBUILD
# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CIBUILD" = "true" ]; then \
  echo "http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports" > /etc/apt/mirrorlist && \
  sed -i -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist#g' /etc/apt/sources.list; \
  fi

ENV DEBIAN_FRONTEND=noninteractive
RUN apt update

FROM baseimage AS nosys-seccomp
COPY nosys-seccomp/nosys.c   /tmp/nosys.c
COPY nosys-seccomp/nosys.sym /tmp/nosys.sym
RUN apt install --no-install-recommends -y gcc libc6-dev libseccomp-dev
RUN gcc -pipe -Wall -Wextra -O2 -shared -fPIC -Wl,--version-script=/tmp/nosys.sym -o /tmp/nosys.so /tmp/nosys.c -lseccomp

############################################
#  Preparation stage: extract and cleanup  #
############################################

FROM baseimage AS extract
ARG WITH_JMX
ARG PYTHON_VERSION
ARG DD_AGENT_ARTIFACT=datadog-agent*_arm64.deb
ARG GENERAL_ARTIFACTS_CACHE_BUCKET_URL

# copy everything - globbing with args wont work
COPY datadog-agent*_arm64.deb /
WORKDIR /output

# Get s6-overlay
ENV S6_VERSION v1.22.1.0
# To calculate S6_SHA256SUM for a specific version, run:
# curl -L https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-aarch64.tar.gz | sha256sum
ENV S6_SHA256SUM=4eac8bfebdb004eaa0b5ff6a09eb0b24e308cc0d7f37912ab19d3d063be3279c
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${GENERAL_ARTIFACTS_CACHE_BUCKET_URL:+${GENERAL_ARTIFACTS_CACHE_BUCKET_URL}/s6-overlay}
ENV JUST_CONTAINERS_DOWNLOAD_LOCATION=${JUST_CONTAINERS_DOWNLOAD_LOCATION:-https://github.com/just-containers/s6-overlay/releases/download}
ADD ${JUST_CONTAINERS_DOWNLOAD_LOCATION}/${S6_VERSION}/s6-overlay-aarch64.tar.gz /output/s6.tgz
RUN echo "${S6_SHA256SUM} /output/s6.tgz" | sha256sum -c -

# Extract and cleanup:
#   - unused systemd unit
#   - GPL sources for embedded software  # FIXME: move upstream
#   - docs and manpages                  # FIXME: move upstream
#   - static libraries                   # FIXME: move upstream
#   - jmxfetch on nojmx build

# DEBUGGING
RUN find / -maxdepth 1 -type f -name 'datadog-agent*_arm64.deb' ! -name "$DD_AGENT_ARTIFACT" -exec rm {} \; \
 && find / -maxdepth 1 -name 'datadog-agent*_arm64.deb' -exec dpkg -X {} . \; | egrep '.*' \
 && rm -rf usr etc/init lib \
    opt/datadog-agent/sources \
    opt/datadog-agent/embedded/share/doc \
    opt/datadog-agent/embedded/share/man \
    # remove libcurl 4.4.0 - it gets removed later anyway but the docker store scanner doesn't see that
    opt/datadog-agent/embedded/lib/libcurl.so.4.4.0 \
    # ditto for this older libsystemd
    usr/lib/x86_64-linux-gnu/libsystemd.so.0.21.0 \
    # self-test certificates that are detected (false positive) as private keys
    opt/datadog-agent/embedded/lib/python*/site-packages/future/backports/test \
 && if [ "$PYTHON_VERSION" = "2" ]; then \
        rm -rf \
            opt/datadog-agent/embedded/bin/2to3-3* \
            opt/datadog-agent/embedded/bin/easy_install-3* \
            opt/datadog-agent/embedded/bin/idle* \
            opt/datadog-agent/embedded/bin/pip3* \
            opt/datadog-agent/embedded/bin/pydoc* \
            opt/datadog-agent/embedded/bin/python3* \
            opt/datadog-agent/embedded/bin/pyvenv* \
            opt/datadog-agent/embedded/include/python3* \
            opt/datadog-agent/embedded/lib/*python3* || true ;\
    fi \
 && if [ "$PYTHON_VERSION" = "3" ]; then \
        rm -rf \
            opt/datadog-agent/embedded/bin/pip2* \
            opt/datadog-agent/embedded/bin/python2* \
            opt/datadog-agent/embedded/include/python2* \
            opt/datadog-agent/embedded/lib/*python2* || true ;\
    fi \
 && find opt/datadog-agent/ -iname "*.a" -delete \
 && if [ -z "$WITH_JMX" ]; then rm -rf opt/datadog-agent/bin/agent/dist/jmx; fi \
 && ln -s /opt/datadog-agent/embedded/ssl etc/ssl \
 && mkdir conf.d checks.d

# Configuration:
#   - copy default config files
COPY datadog*.yaml etc/datadog-agent/

# Installation information
COPY install_info etc/datadog-agent/

######################################
#  Actual docker image construction  #
######################################

FROM baseimage AS release
LABEL maintainer "Datadog <package@datadoghq.com>"
ARG WITH_JMX
ARG PYTHON_VERSION
ENV DOCKER_DD_AGENT=true \
    DD_PYTHON_VERSION=$PYTHON_VERSION \
    PATH=/opt/datadog-agent/bin/agent/:/opt/datadog-agent/embedded/bin/:$PATH \
    CURL_CA_BUNDLE=/opt/datadog-agent/embedded/ssl/certs/cacert.pem \
    # Pass envvar variables to agents
    S6_KEEP_ENV=1 \
    # Direct all agent logs to stdout
    S6_LOGGING=0 \
    # Exit container if entrypoint fails
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    # Allow readonlyrootfs
    S6_READ_ONLY_ROOT=1 \
    # Allow User Group to exec the secret backend script.
    DD_SECRET_BACKEND_COMMAND_ALLOW_GROUP_EXEC_PERM="true"

# make sure we have recent dependencies -- CVE-fixing time!
RUN apt full-upgrade -y \
  # Install iproute2 package for the ss utility that is used by the network check.
  # When the network check will have switched from using ss to directly parsing /proc/net/tcp,
  # this can be removed
  # Install libssl-dev as it's required by some Python checks and we rely on system version
  # Install libseccomp2 as required by `nosys-seccomp` wrapper
  && apt install -y iproute2 libssl-dev libseccomp2 tzdata \
  # https://security-tracker.debian.org/tracker/CVE-2016-2779
  && rm -f /usr/sbin/runuser \
  # https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2018-6954
  && rm -f /usr/lib/x86_64-linux-gnu/libdb-5.3.so

# Install openjdk-11-jre-headless on jmx flavor
RUN if [ -n "$WITH_JMX" ]; then echo "Pulling openjdk-11 from testing" \
 && mkdir -p /usr/share/man/man1 \
 && apt install --no-install-recommends -y openjdk-11-jre-headless \
 && apt clean; fi

# cleaning up
RUN rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Copy agent from extract stage
COPY --from=extract /output/ /

# S6 entrypoint, service definitions, healthcheck probe
COPY s6-services /etc/services.d/
COPY cont-init.d /etc/cont-init.d/
COPY probe.sh initlog.sh secrets-helper/readsecret.py secrets-helper/readsecret.sh secrets-helper/readsecret_multiple_providers.sh /

# Extract s6-overlay
#
# This step is dependant on the distribution's filesystem layout:
# - When Buster moved to merged-usr (/bin/ as a symlink to /usr/bin),
#   we had to change the extraction logic, see #1591
# - The debian image is now built with merged-usr explicitly disabled,
#   see https://github.com/debuerreotype/debuerreotype/pull/50
# - Ubuntu 20.10 uses the symlink /bin -> /usr/bin
RUN tar xzf s6.tgz -C / --exclude="./bin" \
&& tar xzf s6.tgz -C /usr ./bin \
&& rm s6.tgz \
# Prepare for running without root
# - Create a dd-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start
 && adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
 && rm /var/run && mkdir -p /var/run/s6 \
 && chown -R dd-agent:root /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ \
 && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /etc/s6/ /var/run/s6/ /var/log/datadog/ \
 && chmod 755 /probe.sh /initlog.sh \
 && chown root:root /readsecret.py /readsecret.sh /readsecret_multiple_providers.sh \
 && chmod 550 /readsecret.py /readsecret.sh /readsecret_multiple_providers.sh

# Update links to python binaries

RUN if [ -n "$PYTHON_VERSION" ]; then \
 ln -sfn /opt/datadog-agent/embedded/bin/python${PYTHON_VERSION} /opt/datadog-agent/embedded/bin/python \
 && ln -sfn /opt/datadog-agent/embedded/bin/python${PYTHON_VERSION}-config /opt/datadog-agent/embedded/bin/python-config \
 && ln -sfn /opt/datadog-agent/embedded/bin/pip${PYTHON_VERSION} /opt/datadog-agent/embedded/bin/pip ; \
 fi


# Override the exit script by ours to fix --pid=host operations
RUN  mv /etc/s6/init/init-stage3 /etc/s6/init/init-stage3-original
COPY init-stage3          /etc/s6/init/init-stage3
COPY init-stage3-host-pid /etc/s6/init/init-stage3-host-pid

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
COPY --from=nosys-seccomp /tmp/nosys.so /lib/x86_64-linux-gnu/nosys.so
ENV LD_PRELOAD=/lib/x86_64-linux-gnu/nosys.so

# Single entrypoint
COPY entrypoint.sh /bin/entrypoint.sh
COPY entrypoint.d /opt/entrypoints
RUN chmod 755 /bin/entrypoint.sh \
 && chmod 755 -R /opt/entrypoints

CMD ["/bin/entrypoint.sh"]
