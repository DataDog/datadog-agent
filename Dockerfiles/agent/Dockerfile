FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=yes \
    AGENT_VERSION=1:6.0-1 \
    DD_AGENT_HOME=/opt/datadog-agent/

# Install the Agent
COPY datadog-agent*_amd64.deb /datadog-agent*_amd64.deb
RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates\
 && dpkg -i datadog-agent*_amd64.deb\
 && apt-get clean\
 && rm -rf agent6.deb /var/lib/apt/lists/* /tmp/* /var/tmp/*\
 && touch /etc/datadog-agent/datadog.yaml

COPY entrypoint.sh /entrypoint.sh
COPY docker.yaml /etc/dd-agent/conf.d/

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-agent/bin/agent/agent", "start"]
