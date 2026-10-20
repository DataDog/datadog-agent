#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Add bits user to the docker group. This should probably be handled by the base feature. But not working for now.
usermod -aG docker bits
usermod -aG build-shared bits

# Copy lifecycle scripts into the image
install -d /opt/doghome/devcontainer/features/datadog-agent/lifecycle
install -m 755 "$featureDir/lifecycle/postCreate.sh" /opt/doghome/devcontainer/features/datadog-agent/lifecycle/postCreate.sh

# We need to make sure /var/config/dd is populated with base image defaults, since entrypoint is not called in workspaces
rm -rf /var/config/dd
mv /var/config/dd-defaults /var/config/dd

cp /var/config/dd/dd-agent-workspace-env.sh /etc/profile.d/50-agent-workspace-env.sh
# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/zz-ddagent-workspace-env.sh << 'EOF'
export PATH="/home/bits/.local/bin:$PATH" # Make sure we keep it in the path some useful tooling is there
export HOST_ROOT="/host/root"
export HOST_PROC="/host/root/proc"
export HOST_SYS="/host/root/sys"
EOF
