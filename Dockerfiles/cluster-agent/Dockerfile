ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS nosys-seccomp
ENV DEBIAN_FRONTEND=noninteractive
RUN apt update && apt install --no-install-recommends -y gcc libc6-dev libseccomp-dev
COPY nosys-seccomp/nosys.c   /tmp/nosys.c
COPY nosys-seccomp/nosys.sym /tmp/nosys.sym
RUN gcc -pipe -Wall -Wextra -O2 -shared -fPIC -Wl,--version-script=/tmp/nosys.sym -o /tmp/nosys.so /tmp/nosys.c -lseccomp

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS release-base
ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL baseimage.os "ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name "ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name "ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.title "Datadog Cluster Agent"
LABEL org.opencontainers.image.authors="Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.source="https://github.com/DataDog/datadog-agent"
LABEL org.opencontainers.image.vendor="Datadog, Inc."

ARG CI
# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CI" = "true" ]; then \
    echo "http://us-east-1.ec2.archive.ubuntu.com/ubuntu\tpriority:1\nhttp://archive.ubuntu.com/ubuntu" > /etc/apt/mirrorlist.main && \
    echo "http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports" > /etc/apt/mirrorlist.ports && \
    sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
    -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
    -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' /etc/apt/sources.list; \
    fi

ENV PATH="/opt/datadog-agent/bin/:$PATH" \
    DOCKER_DD_AGENT="true"

RUN apt-get update \
    && apt full-upgrade -y \
    && apt-get install --no-install-recommends -y ca-certificates curl libseccomp2 tzdata adduser\
    && rm -rf \
      /var/cache/debconf/*-old \
      /var/lib/apt/lists/* \
      /var/log/* \
      /tmp/* \
      /var/tmp/*

COPY ./conf.d /etc/datadog-agent/conf.d
COPY ./datadog-cluster.yaml /etc/datadog-agent/datadog-cluster.yaml
COPY ./security-agent-policies/compliance/containers/cis-kubernetes*.yaml /etc/datadog-agent/compliance.d/
COPY ./security-agent-policies/compliance/containers/*.rego /etc/datadog-agent/compliance.d/
COPY ./install_info etc/datadog-agent/install_info
COPY ./private-action-runner /etc/datadog-agent/private-action-runner
COPY entrypoint.sh .
COPY readsecret.sh readsecret_multiple_providers.sh ./

####################################
# Actual docker image construction #
####################################

FROM release-base

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA
ARG KUBECTL_VERSION=v1.34.3
ARG KUBECTL_SHA256_AMD64=ab60ca5f0fd60c1eb81b52909e67060e3ba0bd27e55a8ac147cbc2172ff14212
ARG KUBECTL_SHA256_ARM64=46913a7aa0327f6cc2e1cc2775d53c4a2af5e52f7fd8dacbfbfd098e757f19e9
ARG TARGETARCH
ENV \
    # Allow User Group to exec the secret backend script.
    DD_SECRET_BACKEND_COMMAND_ALLOW_GROUP_EXEC_PERM="true" \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL} \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA}

COPY --from=artifacts datadog-cluster-agent /opt/datadog-agent/bin/datadog-cluster-agent
# CWS Instrumentation
COPY --from=artifacts --chmod=0755 ./cws-instrumentation/ /opt/datadog-agent/bin/datadog-cws-instrumentation/
# secret-generic-connector for secret_backend_type / secret_backend_command
COPY --from=artifacts secret-generic-connector /opt/datadog-agent/bin/secret-generic-connector

RUN if [ "${TARGETARCH}" = "amd64" ]; then \
        kubectl_arch="amd64"; \
        kubectl_sha256="${KUBECTL_SHA256_AMD64}"; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        kubectl_arch="arm64"; \
        kubectl_sha256="${KUBECTL_SHA256_ARM64}"; \
    else \
        echo "Unsupported TARGETARCH '${TARGETARCH}' for kubectl install in this POC"; \
        exit 1; \
    fi && \
    curl -fsSLo /usr/local/bin/kubectl-real "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${kubectl_arch}/kubectl" && \
    echo "${kubectl_sha256}  /usr/local/bin/kubectl-real" | sha256sum -c - \
    && printf '%s\n' \
        '#!/bin/sh' \
        'set -eu' \
        '' \
        'REAL_KUBECTL="/usr/local/bin/kubectl-real"' \
        'SA_DIR="/var/run/secrets/kubernetes.io/serviceaccount"' \
        'TOKEN_FILE="${SA_DIR}/token"' \
        'CA_FILE="${SA_DIR}/ca.crt"' \
        'CONFIG_FILE="/tmp/kubectl-incluster-config"' \
        'SERVER="https://kubernetes.default.svc:443"' \
        '' \
        'has_connection_flag=0' \
        'for arg in "$@"; do' \
        '    case "$arg" in' \
        '        --kubeconfig|--kubeconfig=*|--server|--server=*|--context|--context=*|--cluster|--cluster=*|--user|--user=*|--token|--token=*|--certificate-authority|--certificate-authority=*|--client-certificate|--client-certificate=*|--client-key|--client-key=*|--insecure-skip-tls-verify|--insecure-skip-tls-verify=*)' \
        '            has_connection_flag=1' \
        '            break' \
        '            ;;' \
        '    esac' \
        'done' \
        '' \
        'if [ "$has_connection_flag" -eq 0 ] && [ -z "${KUBECONFIG:-}" ] && [ -r "$TOKEN_FILE" ] && [ -r "$CA_FILE" ]; then' \
        "    token=\"\$(tr -d '\\n' < \"\$TOKEN_FILE\")\"" \
        '    if [ -n "$token" ]; then' \
        '        umask 077' \
        '        tmp_config="${CONFIG_FILE}.tmp.$$"' \
        '        cat > "$tmp_config" <<EOK' \
        'apiVersion: v1' \
        'kind: Config' \
        'clusters:' \
        '- name: in-cluster' \
        '  cluster:' \
        '    certificate-authority: ${CA_FILE}' \
        '    server: ${SERVER}' \
        'users:' \
        '- name: in-cluster' \
        '  user:' \
        '    token: ${token}' \
        'contexts:' \
        '- name: in-cluster' \
        '  context:' \
        '    cluster: in-cluster' \
        '    user: in-cluster' \
        'current-context: in-cluster' \
        'EOK' \
        '        mv "$tmp_config" "$CONFIG_FILE"' \
        '        export KUBECONFIG="$CONFIG_FILE"' \
        '    fi' \
        'fi' \
        '' \
        'exec "$REAL_KUBECTL" "$@"' \
    > /usr/local/bin/kubectl \
    && chmod 0755 /usr/local/bin/kubectl-real /usr/local/bin/kubectl \
    && kubectl version --client=true --output=yaml >/dev/null

RUN addgroup --system secret-manager \
    && chmod 755 entrypoint.sh \
    && chown root:secret-manager readsecret.sh readsecret_multiple_providers.sh /opt/datadog-agent/bin/secret-generic-connector \
    && chmod 550 readsecret.sh readsecret_multiple_providers.sh /opt/datadog-agent/bin/secret-generic-connector \
    && chmod g+r,g+w,g+X -R /etc/datadog-agent/ \
    && chmod +x /opt/datadog-agent/bin/datadog-cluster-agent \
    && ln -s /opt/datadog-agent/bin/datadog-cluster-agent /opt/datadog-agent/bin/agent

# Allow running as an unprivileged user:
# - General case is the dd-agent user
# - OpenShift uses a random UID in the root group
#
# Containerd does not preserve permissions when mounting a volume on top
# of an empty folder. Creating .placeholder files as a workaround.
#
RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
    && addgroup --system secret-manager \
    && usermod -a -G secret-manager dd-agent \
    && adduser --system --no-create-home --disabled-password dd-scriptuser --shell /bin/sh \
    && sed -i '/@include common-auth/i auth sufficient pam_succeed_if.so user = dd-scriptuser ruser = dd-agent' /etc/pam.d/su \
    && mkdir -p /var/log/datadog/ /conf.d /opt/datadog-agent/run \
    && touch /var/log/datadog/.placeholder \
    && touch /tmp/.placeholder \
    && chown -R dd-agent:root /etc/datadog-agent/ /var/log/datadog/ /conf.d /tmp/ /opt/datadog-agent/run \
    && chmod g+r,g+w,g+X -R /var/log/datadog/ /conf.d /tmp/ /opt/datadog-agent/run \
    && chmod g+r,g-w,g+X,o-w -R /etc/datadog-agent/ \
    && chmod u=r,g=,o= /etc/datadog-agent/private-action-runner/*

# Ensure the glibc doesn't try to call syscalls that may not be supported
COPY --from=nosys-seccomp /tmp/nosys.so /lib/x86_64-linux-gnu/nosys.so
ENV LD_PRELOAD=/lib/x86_64-linux-gnu/nosys.so

# Incompatible with the custom metrics API on port 443
# Set DD_EXTERNAL_METRICS_PROVIDER_PORT to a higher value to run as non-root
# USER dd-agent

# Leave following directories RW to allow use of readonly rootfs
VOLUME ["/etc/datadog-agent", "/var/log/datadog", "/tmp"]

ENTRYPOINT ["/entrypoint.sh"]

# No docker healthcheck, use a HTTP check
# on port 5005 and/or 443 on Kubernetes

CMD ["datadog-cluster-agent", "start"]
