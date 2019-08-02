##########################################
# Preparation stage: extract and cleanup #
##########################################

FROM debian:buster-slim as extract
ARG WITH_JMX
COPY stackstate-agent*_amd64.deb /
WORKDIR /output

# Get s6-overlay and check gpg signature
ENV S6_VERSION v1.21.2.2
ADD https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-amd64.tar.gz /output/s6.tgz
ADD https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-amd64.tar.gz.sig /tmp/s6.tgz.sig
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
RUN dpkg -x /stackstate-agent*_amd64.deb . \
 && rm -rf usr etc/init lib \
    opt/stackstate-agent/sources \
    opt/stackstate-agent/embedded/share/doc \
    opt/stackstate-agent/embedded/share/man \
    # remove libcurl 4.4.0 - it gets removed later anyway but the docker store scanner doesn't see that
    opt/stackstate-agent/embedded/lib/libcurl.so.4.4.0 \
    # ditto for this older libsystemd
    usr/lib/x86_64-linux-gnu/libsystemd.so.0.21.0 \
 && find opt/stackstate-agent/ -iname "*.a" -delete \
 && if [ -z "$WITH_JMX" ]; then rm -rf opt/stackstate-agent/bin/agent/dist/jmx; fi \
 && ln -s /opt/stackstate-agent/embedded/ssl etc/ssl \
 && mkdir conf.d checks.d

# Configuration:
#   - copy default config files
COPY stackstate*.yaml etc/stackstate-agent/


####################################
# Actual docker image construction #
####################################

FROM debian:buster-slim
LABEL maintainer "StackState <info@stackstate.com>"
ARG WITH_JMX
ENV DOCKER_STS_AGENT=true \
    PATH=/opt/stackstate-agent/bin/agent/:/opt/stackstate-agent/embedded/bin/:$PATH \
    CURL_CA_BUNDLE=/opt/stackstate-agent/embedded/ssl/certs/cacert.pem \
    # Pass envvar variables to agents
    S6_KEEP_ENV=1 \
    # Direct all agent logs to stdout
    S6_LOGGING=0 \
    # Exit container if entrypoint fails
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    # Allow readonlyrootfs
    S6_READ_ONLY_ROOT=1


# Install openjdk-8-jre-headless on jmx flavor
RUN if [ -n "$WITH_JMX" ]; then apt-get update \
 && mkdir /usr/share/man/man1 \
 && apt-get install --no-install-recommends -y openjdk-8-jre-headless \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*; fi

# make sure we have recent dependencies
RUN apt-get update \
  # CVE-fixing time!
  && apt-get install -y util-linux ncurses-bin ncurses-base libncursesw5:amd64 \
  # https://security-tracker.debian.org/tracker/CVE-2018-15686
  && apt-get install -y libudev1 libsystemd0 \
  # https://security-tracker.debian.org/tracker/CVE-2016-2779
  && rm -f /usr/sbin/runuser \
  # https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2018-6954
  && rm -f /usr/lib/x86_64-linux-gnu/libdb-5.3.so \
  && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Copy agent from extract stage
COPY --from=extract /output/ /

# S6 entrypoint, service definitions, healthcheck probe
COPY s6-services /etc/services.d/

# STS: We do not use the dd network agent embedded in the main agent (we use our tcptracer-bpf which is embedded in the process-agent)
RUN rm -rf /etc/services.d/network

COPY entrypoint /etc/cont-init.d/
COPY probe.sh initlog.sh secrets-helper/readsecret.py /

# Extract s6-overlay
# Buster used to have /bin/ symlinked to /usr/bin, which broke
# the tar invocation, see #1591 for the changes we made then
# Is is now back to a dedicated folder
RUN tar xzf s6.tgz \
 && rm s6.tgz \
# Prepare for running without root
# - Create a stackstate-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start
 && adduser --system --no-create-home --disabled-password --ingroup root stackstate-agent \
 && rm /var/run && mkdir -p /var/run/s6 \
 && chown -R stackstate-agent:root /etc/stackstate-agent/ /etc/s6/ /var/run/s6/ /var/log/stackstate-agent/ \
 && chmod g+r,g+w,g+X -R /etc/stackstate-agent/ /etc/s6/ /var/run/s6/ /var/log/stackstate-agent/ \
 && chmod 755 /probe.sh /initlog.sh \
 && chown root:root /readsecret.py \
 && chmod 500 /readsecret.py

# Override the exit script by ours to fix --pid=host operations
COPY init-stage3 /etc/s6/init/init-stage3

# Expose DogStatsD and trace-agent ports
EXPOSE 8125/udp 8126/tcp

HEALTHCHECK --interval=2m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

# Leave following directories RW to allow use of kubernetes readonlyrootfs flag
VOLUME ["/var/run/s6", "/etc/stackstate-agent", "/var/log/stackstate", "/tmp"]

CMD ["/init"]
