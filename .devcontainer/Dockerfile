FROM qmcgaw/godevcontainer:debian

USER root

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

# Revert configurations that was set at top layer (for avoiding build warnings and errors).
ENV DEBIAN_FRONTEND=dialog

# Expose service ports.
EXPOSE 8000

USER vscode
