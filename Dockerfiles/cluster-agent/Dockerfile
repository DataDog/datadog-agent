########################################
# Preparation stage: layout and chmods #
########################################

FROM debian:buster-slim as builder

WORKDIR /output

RUN mkdir -p opt/datadog-agent/bin/datadog-cluster-agent/

COPY datadog-cluster-agent opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent
COPY ./conf.d etc/datadog-agent/conf.d
COPY ./datadog-cluster.yaml etc/datadog-agent/datadog-cluster.yaml
COPY ./dist opt/datadog-agent/bin/datadog-cluster-agent/dist
COPY entrypoint.sh .

RUN chmod 755 entrypoint.sh \
    && chmod g+r,g+w,g+X -R etc/datadog-agent/ \
    && chmod +x opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent

####################################
# Actual docker image construction #
####################################

FROM debian:buster-slim

LABEL maintainer "Datadog <package@datadoghq.com>"

ENV PATH="/opt/datadog-agent/bin/datadog-cluster-agent/:/opt/datadog-agent/embedded/bin/:$PATH"

RUN apt-get update \
 && apt-get install --no-install-recommends -y ca-certificates curl \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=builder /output /

ENTRYPOINT ["/entrypoint.sh"]

# No docker healthcheck, use a HTTP check
# on port 5005 and/or 443 on Kubernetes

CMD ["datadog-cluster-agent", "start"]
