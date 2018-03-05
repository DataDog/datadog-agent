FROM debian:stretch-slim
COPY datadog-cluster-agent /

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=yes \
    AGENT_VERSION=1:6.0-1 \
    DD_AGENT_HOME=/opt/datadog-cluster-agent/

# Install the Agent
RUN mkdir -p /opt/datadog-cluster-agent/bin/datadog-cluster-agent/
COPY datadog-cluster-agent /opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent

RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates\
 && apt-get clean

VOLUME ["/conf.d", "/checks.d"]

COPY entrypoint.sh .

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent", "start"]
