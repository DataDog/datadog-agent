FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DD_AGENT_HOME=/opt/datadog-cluster-agent/

# Install the Agent
RUN mkdir -p /opt/datadog-cluster-agent/bin/datadog-cluster-agent/

COPY datadog-cluster-agent /opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent

COPY ./conf.d /etc/datadog-cluster-agent/etc/conf.d
COPY ./datadog-cluster.yaml /etc/datadog-cluster-agent/etc/datadog-cluster.yaml

RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates\
 && apt-get clean

COPY entrypoint.sh .

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent", "start"]
