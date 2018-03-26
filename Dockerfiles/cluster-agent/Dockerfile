FROM debian:stretch-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV PATH="/opt/datadog-cluster-agent/bin/datadog-cluster-agent/:/opt/datadog-cluster-agent/embedded/bin/":$PATH

# Install the Agent
RUN mkdir -p /opt/datadog-cluster-agent/bin/datadog-cluster-agent/

COPY datadog-cluster-agent /opt/datadog-cluster-agent/bin/datadog-cluster-agent/datadog-cluster-agent

COPY ./conf.d /etc/datadog-cluster-agent/etc/conf.d
COPY ./datadog-cluster.yaml /etc/datadog-cluster-agent/etc/datadog-cluster.yaml
COPY ./dist /opt/datadog-cluster-agent/bin/datadog-cluster-agent/dist

RUN apt-get update \
 && apt-get install --no-install-recommends -y apt-transport-https ca-certificates curl\
 && apt-get clean \
 && apt-get autoremove

COPY entrypoint.sh .

RUN chmod 755 /entrypoint.sh \
    && chmod g+r,g+w,g+X -R /etc/datadog-cluster-agent/

ENTRYPOINT ["/entrypoint.sh"]

CMD ["datadog-cluster-agent", "start"]
