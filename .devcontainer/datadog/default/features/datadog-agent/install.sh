#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Install dda binary from GitHub releases
ARCH=$(uname -m)
if [[ "${ARCH}" == "x86_64" ]]; then
    DDA_ARCH="x86_64-unknown-linux-gnu"
elif [[ "${ARCH}" == "aarch64" ]]; then
    DDA_ARCH="aarch64-unknown-linux-gnu"
else
    echo "Unsupported architecture: ${ARCH}" >&2
    exit 1
fi

curl -fsSL "https://github.com/DataDog/datadog-agent-dev/releases/download/v${DDAVERSION}/dda-${DDA_ARCH}.tar.gz" \
    | tar -xzf - -C /usr/local/bin dda
chmod +x /usr/local/bin/dda

# Add bits user to the docker group
usermod -aG docker bits
newgrp docker

# Copy lifecycle scripts into the image
install -d /opt/doghome/devcontainer/features/datadog-agent/lifecycle
install -m 755 "$featureDir/lifecycle/postCreate.sh" /opt/doghome/devcontainer/features/datadog-agent/lifecycle/postCreate.sh

# Persist the dev env image reference for postCreate.sh
echo "${DEVENVIMAGE}" > /opt/doghome/devcontainer/dev-env-image

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
EOF

# Automatically enter the developer environment on every terminal login
cat >> /home/bits/.zshrc << 'EOF'

# Enter the developer environment automatically
exec dda env dev shell
EOF
