# Use the Ubuntu Slim AMD64 base image
ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
FROM ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS builder
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME
LABEL baseimage.os="ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.title="Otel Agent"

ARG CIBUILD

# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CIBUILD" = "true" ]; then \
  echo "http://us-east-1.ec2.archive.ubuntu.com/ubuntu\tpriority:1\nhttp://archive.ubuntu.com/ubuntu" > /etc/apt/mirrorlist.main && \
  echo "http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports" > /etc/apt/mirrorlist.ports && \
  sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
         -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
         -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' /etc/apt/sources.list; \
  fi

# Set environment variables
ARG DD_GIT_COMMIT_SHA
ENV DEBIAN_FRONTEND=noninteractive

# Set the working directory
WORKDIR /workspace

# Update and install necessary packages
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    software-properties-common \
    build-essential \
    git \
    python3 \
    python3-pip \
    python3-venv

# TODO: For now we have to clone the whole repository and checkout to a specific commit / ref / branch then recompile the otel-agent
# Cloning the whole repo is necessary because `get_version` in the build script runs git describe --tags --candidates=50...
# We should eventually just fetch the binary and just copy it
RUN git clone --filter=blob:none https://github.com/DataDog/datadog-agent.git datadog-agent && \
    cd datadog-agent && \
    git checkout $DD_GIT_COMMIT_SHA

RUN ls /workspace/

# Set the working directory to the source code
WORKDIR /workspace/datadog-agent

# Install Go based on architecture
RUN GO_VERSION=$(cat .go-version) && \
    ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then \
    GO_ARCH="linux-amd64"; \
    elif [ "$ARCH" = "arm64" ]; then \
    GO_ARCH="linux-arm64"; \
    else \
    echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    curl -OL https://golang.org/dl/go${GO_VERSION}.$GO_ARCH.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.$GO_ARCH.tar.gz && \
    rm go${GO_VERSION}.$GO_ARCH.tar.gz

# Set up Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
RUN mkdir /go
ENV GOPATH=/go

# Verify installations
RUN go version && \
    python3 --version && \
    curl --version

# Create and activate virtual environment, then install requirements
RUN python3 -m venv venv && \
    . venv/bin/activate && \
    pip install --upgrade pip && \
    pip install -r requirements.txt

# TODO confirm if we can remove this
# Copy the manifest file
# COPY manifest.yaml /workspace/datadog-agent/comp/otelcol/collector-contrib/impl/manifest.yaml

# Generate the files
RUN . venv/bin/activate && invoke collector.generate

# Build the OTel agent
RUN . venv/bin/activate && invoke otel-agent.build

# ------------------------------
# Final stage
# ------------------------------

FROM ubuntu:$BASE_IMAGE_UBUNTU_VERSION as release

ARG DD_GIT_REPOSITORY_URL
ARG DD_GIT_COMMIT_SHA
LABEL maintainer="Datadog <package@datadoghq.com>"

ENV DEBIAN_FRONTEND=noninteractive
# make sure we have recent dependencies -- CVE-fixing time!
RUN apt update && \
    apt full-upgrade -y && \
    apt install -y tzdata ca-certificates adduser && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

# Copy the built OTel agent from the builder stage
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/otel-agent /opt/datadog-agent/embedded/bin/otel-agent
# TODO: confirm if this should be copied to otel-config.yaml.example
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/dist/otel-config.yaml /etc/datadog-agent/otel-config.yaml

# Prepare for running without root
# - Create a dd-agent:root user and give it permissions on relevant folders
# - Remove the /var/run -> /run symlink and create a legit /var/run folder
# as some docker versions re-create /run from zero at container start
RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent \
  && rm /var/run && mkdir -p /var/run/datadog /var/log/datadog/ \
  && chown -R dd-agent:root /etc/datadog-agent/ /var/log/datadog/ /var/run/datadog/ \
  && chmod g+r,g+w,g+X -R /etc/datadog-agent/ /var/log/datadog/ /var/run/datadog/

# Check that the UID of dd-agent is still 100.
#
# The exact numeric value of the UID of the dd-agent user shouldn’t matter.
# But people that don’t want to let the agent run as root might want to explicitly set a non-root user in their k8s security context.
# And, in a k8s security context, we can only specify a numeric UID and not a username.
# So, if the UID of the dd-agent user happen to change again and cannot be forced to 100 (because of a conflict), we need to update
# * the documentation https://docs.datadoghq.com/data_security/kubernetes/#running-container-as-root-user
#   (see PR https://github.com/DataDog/documentation/pull/21889)
# * https://datadoghq.atlassian.net/wiki/spaces/TS/pages/2615709591/Why+the+containerized+Agent+runs+as+root#Agent-user
RUN [ "$(getent passwd dd-agent | cut -d: -f 3)" -eq 100 ]

RUN find /etc -type d,f -perm -o+w -print0 | xargs -r -0 chmod g-w,o-w

ENV PATH="/opt/datadog-agent/embedded/bin:${PATH}" \
    DD_GIT_COMMIT_SHA=${DD_GIT_COMMIT_SHA} \
    DD_GIT_REPOSITORY_URL=${DD_GIT_REPOSITORY_URL}

ENTRYPOINT ["/bin/bash"]
CMD ["/opt/datadog-agent/embedded/bin/otel-agent", "run"]
