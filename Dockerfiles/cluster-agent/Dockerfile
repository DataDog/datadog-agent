########################################
# Preparation stage: layout and chmods #
########################################

FROM debian:buster-slim as builder

WORKDIR /output

COPY stackstate-cluster-agent opt/stackstate-agent/bin/stackstate-cluster-agent
COPY ./conf.d etc/stackstate-agent/conf.d
COPY ./stackstate-cluster.yaml etc/stackstate-agent/stackstate-cluster.yaml
COPY ./dist/templates opt/stackstate-agent/bin/dist/templates
COPY entrypoint.sh .

RUN chmod 755 entrypoint.sh \
    && chmod g+r,g+w,g+X -R etc/stackstate-agent/ \
    && chmod +x opt/stackstate-agent/bin/stackstate-cluster-agent \
    && ln -s /opt/stackstate-agent/bin/stackstate-cluster-agent opt/stackstate-agent/bin/agent

####################################
# Actual docker image construction #
####################################

FROM debian:buster-slim

LABEL maintainer "StackState <info@stackstate.com>"

ENV PATH="/opt/stackstate-agent/bin/:$PATH"

RUN apt-get update \
 && apt-get install --no-install-recommends -y ca-certificates curl \
 # https://security-tracker.debian.org/tracker/CVE-2018-15686
 && apt-get install -y libudev1 libsystemd0 \
 && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=builder /output /

# Allow running as an unprivileged user:
# - General case is the stackstate-agent user
# - OpenShift uses a random UID in the root group
#
# Containerd does not preserve permissions when mounting a volume on top
# of an empty folder. Creating .placeholder files as a workaround.
#
RUN adduser --system --no-create-home --disabled-password --ingroup root stackstate-agent \
 && mkdir -p /var/log/stackstate/ /conf.d \
 && touch /var/log/stackstate/.placeholder \
 && touch /tmp/.placeholder \
 && chown -R stackstate-agent:root /etc/stackstate-agent/ /var/log/stackstate/ /conf.d /tmp/ \
 && chmod g+r,g+w,g+X -R /etc/stackstate-agent/ /var/log/stackstate/ /conf.d /tmp/

# Incompatible with the custom metrics API on port 443
# Set STS_EXTERNAL_METRICS_PROVIDER_PORT to a higher value to run as non-root
# USER stackstate-agent

# Leave following directories RW to allow use of readonly rootfs
VOLUME ["/etc/stackstate-agent", "/var/log/stackstate", "/tmp"]

ENTRYPOINT ["/entrypoint.sh"]

# No docker healthcheck, use a HTTP check
# on port 5005 and/or 443 on Kubernetes

CMD ["stackstate-cluster-agent", "start"]
