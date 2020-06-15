############################################
#  Preparation stage: extract and cleanup  #
############################################

FROM debian:bullseye-slim AS extract
ARG WITH_JMX
ARG PYTHON_VERSION
ARG DD_AGENT_ARTIFACT=datadog-agent*_arm64.deb

# copy everything - globbing with args wont work
COPY datadog-agent*_arm64.deb /
WORKDIR /output

# Get s6-overlay and check gpg signature
ENV S6_VERSION v1.22.1.0
ADD https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-aarch64.tar.gz /output/s6.tgz
ADD https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-aarch64.tar.gz.sig /tmp/s6.tgz.sig
RUN apt-get update \
 && apt-get install --no-install-recommends -y gpg gpg-agent curl ca-certificates \
 && curl https://keybase.io/justcontainers/key.asc | gpg --import \
 && gpg --verify /tmp/s6.tgz.sig /output/s6.tgz

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
    opt/datadog-agent/embedded/lib/python*/site-packages/Cryptodome/SelfTest \
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

FROM debian:bullseye-slim AS release
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
    S6_READ_ONLY_ROOT=1

# make sure we have recent dependencies
RUN apt-get update \
  # CVE-fixing time!
  && apt-get install -y util-linux ncurses-bin ncurses-base libncursesw5:arm64 \
  # https://security-tracker.debian.org/tracker/CVE-2018-15686
  && apt-get install -y libudev1 libsystemd0 \
  # https://security-tracker.debian.org/tracker/CVE-2016-2779
  && rm -f /usr/sbin/runuser \
  # https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2018-6954
  && rm -f /usr/lib/x86_64-linux-gnu/libdb-5.3.so

# Install openjdk-11-jre-headless on jmx flavor
# Due to this bug https://bugs.openjdk.java.net/browse/JDK-8217766, we need to be able to
# pull from testing since this is the only place for now where a version > 11.0.4 is available.
# we leave testing in the sources to avoid dependencies conflict in custom images
RUN if [ -n "$WITH_JMX" ]; then echo "Pulling openjdk-11 from testing" \
 && echo "deb http://deb.debian.org/debian testing main" > /etc/apt/sources.list.d/testing.list \
 && apt-get update \
 && mkdir /usr/share/man/man1 \
 && apt-get install --no-install-recommends -y openjdk-11-jre-headless \
 && apt-get clean; fi

# cleaning up
RUN rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Copy agent from extract stage
COPY --from=extract /output/ /

# S6 entrypoint, service definitions, healthcheck probe
COPY s6-services /etc/services.d/
COPY entrypoint /etc/cont-init.d/
COPY probe.sh initlog.sh secrets-helper/readsecret.py /

# Extract s6-overlay
#
# This step is dependant on the distribution's filesystem layout:
# - When Buster moved to merged-usr (/bin/ as a symlink to /usr/bin),
#   we had to change the extraction logic, see #1591
# - The debian image is now built with merged-usr explicitly disabled,
#   see https://github.com/debuerreotype/debuerreotype/pull/50
RUN tar xzf s6.tgz \
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
 && chown root:root /readsecret.py \
 && chmod 500 /readsecret.py

# Update links to python binaries

RUN if [ ! -z "$PYTHON_VERSION" ]; then \
 ln -sfn /opt/datadog-agent/embedded/bin/python${PYTHON_VERSION} /opt/datadog-agent/embedded/bin/python \
 && ln -sfn /opt/datadog-agent/embedded/bin/python${PYTHON_VERSION}-config /opt/datadog-agent/embedded/bin/python-config \
 && ln -sfn /opt/datadog-agent/embedded/bin/pip${PYTHON_VERSION} /opt/datadog-agent/embedded/bin/pip ; \
 fi


# Override the exit script by ours to fix --pid=host operations
RUN  mv /etc/s6/init/init-stage3 /etc/s6/init/init-stage3-original
COPY init-stage3          /etc/s6/init/init-stage3
COPY init-stage3-host-pid /etc/s6/init/init-stage3-host-pid

# Expose DogStatsD and trace-agent ports
EXPOSE 8125/udp 8126/tcp

HEALTHCHECK --interval=30s --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

# Leave following directories RW to allow use of kubernetes readonlyrootfs flag
VOLUME ["/var/run/s6", "/etc/datadog-agent", "/var/log/datadog", "/tmp"]

CMD ["/init"]


################################################################
#  Sanity checks on the image contents                         #
#  Build the release artifact with "--target release" to skip  #
################################################################

FROM release AS testing
ARG WITH_JMX

COPY test_*.py /
RUN python /test_image_contents.py -v
