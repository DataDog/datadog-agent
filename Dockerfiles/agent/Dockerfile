FROM debian:stretch-slim as extract
ARG WITH_JMX
COPY datadog-agent*_amd64.deb /
WORKDIR /output

# Extract and cleanup:
#   - unused systemd unit
#   - GPL sources for embedded software  # FIXME: move upstream
#   - docs and manpages                  # FIXME: move upstream
#   - static libraries                   # FIXME: move upstream
#   - jmxfetch on nojmx build
RUN dpkg -x /datadog-agent*_amd64.deb . \
 && echo "Cleanup and trimming" \
 && rm -rf usr etc/init lib \
    opt/datadog-agent/sources \
    opt/datadog-agent/embedded/share/doc \
    opt/datadog-agent/embedded/share/man \
 && find opt/datadog-agent/ -iname "*.a" -delete \
 && if [ -z "$WITH_JMX" ]; then rm -rf opt/datadog-agent/bin/agent/dist/jmx; fi \
 && echo "Linking embedded ssl certificates" \
 && mkdir -p etc \
 && ln -s /opt/datadog-agent/embedded/ssl etc/ssl

# Configuration:
#   - empty config file to remove logline
#   - enable docker check by default
#   - enable process agent
COPY datadog.yaml etc/datadog-agent/datadog.yaml
RUN mv etc/datadog-agent/conf.d/docker.yaml.example etc/datadog-agent/conf.d/docker.yaml.default \
 && sed -i "s/enabled: false/enabled: true/" etc/datadog-agent/conf.d/process_agent.yaml.default
COPY entrypoint.sh .


FROM debian:stretch-slim
LABEL maintainer "Datadog <package@datadoghq.com>"
ARG WITH_JMX
ENV DOCKER_DD_AGENT=yes \
    PATH=/opt/datadog-agent/bin/agent/:/opt/datadog-agent/embedded/bin/:$PATH \
    CURL_CA_BUNDLE=/opt/datadog-agent/embedded/ssl/certs/cacert.pem

# Install openjdk-8-jre-headless on jmx flavor
RUN if [ -n "$WITH_JMX" ]; then apt-get update \
 && mkdir /usr/share/man/man1 \
 && apt-get install --no-install-recommends -y openjdk-8-jre-headless \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*; fi

# Copy agent from extract stage
COPY --from=extract /output/ /

# Expose DogStatsD and trace-agent ports
EXPOSE 8125/udp 8126/tcp

# Placeholder healthcheck, to be improved
HEALTHCHECK --interval=5m --timeout=20s --retries=1 \
  CMD ["/opt/datadog-agent/bin/agent/agent", "status"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/opt/datadog-agent/bin/agent/agent", "start"]
