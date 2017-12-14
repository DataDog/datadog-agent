FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=yes \
    AGENT_VERSION=1:7.0-1 \
    DD_AGENT_HOME=/opt/datadog-agent/

# Install the Agent
COPY bin/cluster-agent/cluster-agent $DD_AGENT_HOME/bin/cluster-agent/

RUN apt-get update \
 && apt-get install --no-install-recommends -y libpython2.7 curl vim apt-transport-https ca-certificates\
 && apt-get clean

COPY Dockerfiles/clusteragent/entrypoint.sh /entrypoint.sh
COPY dev/dist/datadog.yaml $DD_AGENT_HOME/dev/dist/

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-agent/bin/cluster-agent/cluster-agent", "start"]