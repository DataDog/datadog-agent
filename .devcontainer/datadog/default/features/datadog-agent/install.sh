#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Get claude from the buildimages /root/.local/bin
cp /root/.local/bin/claude /home/bits/.local/bin/claude

# Ensure bits is in the docker and build-shared groups. Both are pre-baked in the
# dev-env-workspace image; these guards are a no-op in practice but make the feature
# safe to apply on other bases where the groups may be absent.
getent group docker >/dev/null && usermod -aG docker bits
getent group build-shared >/dev/null && usermod -aG build-shared bits

# Copy lifecycle scripts into the image
install -d /opt/doghome/devcontainer/features/datadog-agent/lifecycle
install -m 755 "$featureDir/lifecycle/postCreate.sh" /opt/doghome/devcontainer/features/datadog-agent/lifecycle/postCreate.sh

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
EOF
