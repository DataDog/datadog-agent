#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Get claude from the buildimages /root/.local/bin
cp /root/.local/bin/claude /home/bits/.local/bin/claude

# Add bits user to the docker group. This should probably be handled by the base feature. But not working for now.
usermod -aG docker bits

# Copy lifecycle scripts into the image
install -d /opt/doghome/devcontainer/features/datadog-agent/lifecycle
install -m 755 "$featureDir/lifecycle/postCreate.sh" /opt/doghome/devcontainer/features/datadog-agent/lifecycle/postCreate.sh

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
EOF
