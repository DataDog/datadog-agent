
####################################
# Actual docker image construction #
####################################

FROM debian:buster-slim
LABEL maintainer "StackState <info@stackstate.com>"
ENV DOCKER_STS_AGENT=true \
    PATH=/opt/stackstate-agent/bin/agent/:/opt/stackstate-agent/embedded/bin/:$PATH \
    CURL_CA_BUNDLE=/opt/stackstate-agent/embedded/ssl/certs/cacert.pem \
    # Enable trace agent
    STS_APM_ENABLED=true
COPY trace-agent* /opt/stackstate-agent/bin/agent/
WORKDIR /opt/stackstate-agent/bin/agent

COPY probe.sh entrypoint/init-trace.sh /

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
  && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
  && mkdir -p /etc/stackstate-agent/ && mkdir -p /var/log/stackstate-agent/

# Prepare for running without root
# - Create a stackstate-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start

RUN  adduser --system --no-create-home --disabled-password --ingroup root stackstate-agent \
 && chown -R stackstate-agent:root /etc/stackstate-agent/ /var/log/stackstate-agent/ \
 && chmod g+r,g+w,g+X -R /etc/stackstate-agent/ /var/log/stackstate-agent/ \
 && chmod 755 /probe.sh /init-trace.sh


# Copy default configuration files
COPY stackstate*.yaml /etc/stackstate-agent/

# Expose trace-agent port
EXPOSE  8126/tcp

HEALTHCHECK --interval=2m --timeout=5s --retries=2 \
  CMD ["/probe.sh"]

# Leave following directories RW to allow use of kubernetes readonlyrootfs flag
VOLUME ["/etc/stackstate-agent", "/var/log/stackstate", "/tmp"]

CMD ["/init-trace.sh"]
