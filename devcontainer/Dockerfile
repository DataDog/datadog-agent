FROM registry.ddbuild.io/ci/datadog-agent-devenv:1-amd64

USER root
# Add the dog group (gid 501) and bits user (uid 2000) to match the host workspace identity.
# /home/datadog/go/bin is 755 so bits can reach pre-installed tools (golangci-lint, dlv, etc.).
RUN groupadd -g 501 dog && \
    useradd -m -u 2000 -g 501 -G sudo -s /bin/bash bits && \
    echo 'bits ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers && \
    git config --system --add safe.directory /workspace

# Tools that are dynamically linked and must live in the image.
# gh/claude/ddtool/dd-gitsign are statically linked and bind-mounted at runtime instead.
RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux pass gpg && \
    rm -rf /var/lib/apt/lists/*

USER bits
ENV HOME=/home/bits \
    GOPATH=/home/bits/go \
    PATH="/home/bits/go/bin:/home/datadog/go/bin:/usr/local/go/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
WORKDIR /workspace
