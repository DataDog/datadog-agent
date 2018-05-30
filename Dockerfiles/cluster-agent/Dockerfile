FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV PATH="/opt/datadog-agent/bin/datadog-cluster-agent/:/opt/datadog-agent/embedded/bin/":$PATH

# Install the Agent
RUN mkdir -p /opt/datadog-agent/bin/datadog-cluster-agent/

COPY datadog-cluster-agent /opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent

COPY ./conf.d /etc/datadog-agent/conf.d
COPY ./datadog-cluster.yaml /etc/datadog-agent/datadog.yaml
COPY ./dist /opt/datadog-agent/bin/datadog-cluster-agent/dist

RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates curl\
 && apt-get clean \
 && apt-get autoremove

COPY entrypoint.sh .

RUN chmod 755 /entrypoint.sh \
    && chmod g+r,g+w,g+X -R /etc/datadog-agent/

ENTRYPOINT ["/entrypoint.sh"]

CMD ["datadog-cluster-agent", "start"]
