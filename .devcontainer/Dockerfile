ARG GOLANGCI_LINT_VERSION=v1.37.1
FROM golangci/golangci-lint:${GOLANGCI_LINT_VERSION} as golangci-lint

FROM golang:latest

# Configure to avoid build warnings and errors as described in official VSCode Remote-Containers extension documentation.
# See https://code.visualstudio.com/docs/remote/containers-advanced#_reducing-dockerfile-build-warnings.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -y \
    && apt-get -y install --no-install-recommends apt-utils 2>&1 \
    # Verify git, process tools, lsb-release (common in install instructions for CLIs) installed.
    && apt-get -y install git iproute2 procps lsb-release \
    # Install Python2.7
    && apt-get install -y python2.7 python-pip \
    && apt-get autoremove -y \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

ENV GOPATH=/go
ENV PATH=$GOPATH/bin:/usr/local/go/bin:$PATH
ARG GOPLS_VERSION=v0.6.6
ARG DELVE_VERSION=v1.5.0
ARG GOMODIFYTAGS_VERSION=v1.13.0
ARG GOPLAY_VERSION=v1.0.0
ARG GOTESTS_VERSION=v1.5.3
ARG MOCK_VERSION=v1.5.0
ARG MOCKERY_VERSION=v2.3.0
COPY --from=golangci-lint /usr/bin/golangci-lint ${GOPATH}/bin
RUN go get -v golang.org/x/tools/gopls@${GOPLS_VERSION} 2>&1
RUN go get -v \
    # Base Go tools needed for VS code Go extension
    golang.org/x/tools/cmd/guru \
    golang.org/x/tools/cmd/gorename \
    github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION} \
    github.com/uudashr/gopkgs/v2/cmd/gopkgs@latest \
    github.com/ramya-rao-a/go-outline \
    # Extra tools integrating with VS code
    github.com/fatih/gomodifytags@${GOMODIFYTAGS_VERSION} \
    github.com/haya14busa/goplay/cmd/goplay@${GOPLAY_VERSION} \
    github.com/cweill/gotests/...@${GOTESTS_VERSION} \
    github.com/davidrjenni/reftools/cmd/fillstruct \
    # Terminal tools
    github.com/golang/mock/gomock@${MOCK_VERSION} \
    github.com/golang/mock/mockgen@${MOCK_VERSION} \
    github.com/vektra/mockery/v2/...@${MOCKERY_VERSION} \
    2>&1

# EXTRA TOOLS
# Kubectl
ARG KUBECTL_VERSION=v1.19.4
RUN wget -qO /usr/local/bin/kubectl "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    chmod 755 /usr/local/bin/kubectl

# Stern
ARG STERN_VERSION=1.11.0
RUN wget -qO /usr/local/bin/stern https://github.com/wercker/stern/releases/download/${STERN_VERSION}/stern_$(uname -s)_amd64 && \
    chown ${USER_UID}:${USER_GID} /usr/local/bin/stern && \
    chmod 755 /usr/local/bin/stern

# Kubectx and Kubens
ARG KUBECTX_VERSION=v0.9.3
RUN wget -qO- "https://github.com/ahmetb/kubectx/releases/download/${KUBECTX_VERSION}/kubectx_${KUBECTX_VERSION}_$(uname -s)_$(uname -m).tar.gz" | \
    tar -xzC /usr/local/bin kubectx && \
    wget -qO- "https://github.com/ahmetb/kubectx/releases/download/${KUBECTX_VERSION}/kubens_${KUBECTX_VERSION}_$(uname -s)_$(uname -m).tar.gz" | \
    tar -xzC /usr/local/bin kubens && \
    chmod 755 /usr/local/bin/kube*

# Helm
ARG HELM3_VERSION=v3.5.2
RUN wget -qO- "https://get.helm.sh/helm-${HELM3_VERSION}-linux-amd64.tar.gz" | \
    tar -xzC /usr/local/bin --strip-components=1 linux-amd64/helm && \
    chmod 755 /usr/local/bin/helm*

# Revert configurations that was set at top layer (for avoiding build warnings and errors).
ENV DEBIAN_FRONTEND=dialog

# Expose service ports.
EXPOSE 8000

