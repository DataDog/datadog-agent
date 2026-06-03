#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Get claude from the buildimages /root/.local/bin
cp /root/.local/bin/claude /home/bits/.local/bin/claude

# Add bits to docker and build-shared groups. Both groups are buildimage-specific
# (created in linux/Dockerfile) and absent on stock workspace images; the getent
# guard makes this a clean no-op on stock bases. Runs after `installsAfter: base`
# ensures bits exists before this feature runs.
# build-shared (GID 9001): grants write access to group-owned toolchain trees
#   (RVM at /opt/dd/rvm, rustup at /opt/dd/rustup, go cache at /var/config/dd/go,
#    conda at /opt/dd/conda). All owned root:build-shared with setgid dirs.
#   The base feature creates bits with primary group `dog` only; without this
#   line bits cannot write into any toolchain cache.
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
