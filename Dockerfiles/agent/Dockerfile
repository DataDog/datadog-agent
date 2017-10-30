FROM debian:stretch-slim as extract
ARG WITH_JMX
COPY datadog-agent*_amd64.deb /
WORKDIR /output

# Extract and cleanup:
#   - unused systemd unit
#   - GPL sources for embedded software
#   - docs and manpages
#   - static libraries
#   - jmxfetch on nojmx build
RUN dpkg -x /datadog-agent*_amd64.deb . \
 && rm -rf usr etc/init lib \
    opt/datadog-agent/sources \
    opt/datadog-agent/embedded/share/doc \
    opt/datadog-agent/embedded/share/man \
 && find opt/datadog-agent/ -iname "*.a" -delete \
 && if [ -z "$WITH_JMX" ]; then rm -rf opt/datadog-agent/bin/agent/dist/jmx; fi

# Configuration:
#   - empty config file to remove logline
#   - enable docker check by default
#   - enable process agent
 RUN touch etc/datadog-agent/datadog.yaml \
 && mv etc/datadog-agent/conf.d/docker.yaml.example etc/datadog-agent/conf.d/docker.yaml.default \
 && sed -i "s/enabled: false/enabled: true/" etc/datadog-agent/conf.d/process_agent.yaml.default
COPY entrypoint.sh .


FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ARG WITH_JMX
ENV DOCKER_DD_AGENT=yes \
    PATH=/opt/datadog-agent/bin/agent/:/opt/datadog-agent/embedded/bin/:$PATH \
    SSL_CERT_DIR=/opt/datadog-agent/embedded/ssl

# Install dependencies:
#   - busybox (top+wget) for support - FIXME: remove once our support tooling has improved
#   - openjdk-8-jre-headless for jmx flavor
RUN apt-get update \
 && apt-get install --no-install-recommends -y busybox \
 && if [ -n "$WITH_JMX" ]; then mkdir /usr/share/man/man1 && apt-get install --no-install-recommends -y openjdk-8-jre-headless; fi \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* \
 && busybox --install

COPY --from=extract /output/ /

EXPOSE 8125/udp
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/opt/datadog-agent/bin/agent/agent", "start"]
