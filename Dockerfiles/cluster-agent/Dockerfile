FROM debian:stretch-slim
COPY datadog-cluster-agent*_amd64.deb /

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV DOCKER_DD_AGENT=yes \
    AGENT_VERSION=1:7.0-1 \
    DD_AGENT_HOME=/opt/datadog-cluster-agent \
    PATH=/opt/datadog-cluster-agent/bin/datadog-cluster-agent/:/opt/datadog-cluster-agent/embedded/bin/:$PATH

# Install the Agent

RUN dpkg -x /datadog-cluster-agent*_amd64.deb .
RUN rm datadog-cluster-agent*_amd64.deb

RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates curl net-tools vim unzip\
 && apt-get clean

COPY entrypoint.sh .

EXPOSE 8125/udp

ENTRYPOINT ["/entrypoint.sh"]

CMD ["/opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent", "start"]
