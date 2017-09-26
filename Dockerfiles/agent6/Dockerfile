FROM debian:jessie

MAINTAINER Datadog <package@datadoghq.com>

ENV DOCKER_DD_AGENT=yes \
    AGENT_VERSION=1:6.0-1 \
    DD_AGENT_HOME=/opt/datadog-agent/

# Install the Agent
RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates\
 && echo "deb https://s3.amazonaws.com/apt-agent6.datad0g.com unstable main" > /etc/apt/sources.list.d/datadog.list \
 && apt-key adv --keyserver keyserver.ubuntu.com --recv-keys A2923DFF56EDA6E76E55E492D3A80E30382E94DE \
 && apt-get update \
 # TODO: pin the version
 && apt-get install --no-install-recommends -y datadog-agent \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY entrypoint.sh /entrypoint.sh
COPY docker.yaml /etc/dd-agent/conf.d/

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-agent/bin/agent/agent", "start"]
