#!/bin/bash
set -euo pipefail
featureDir=$(cd "$(dirname "$0")"; pwd)

# Keep Claude available when the image already carries it.
if [[ -f /root/.local/bin/claude && -x /root/.local/bin/claude ]]; then
    install -d -o bits -g dog /home/bits/.local/bin
    install -m 755 -o bits -g dog /root/.local/bin/claude /home/bits/.local/bin/claude
fi

# Copy lifecycle scripts into the image
install -d /opt/doghome/devcontainer/features/datadog-agent/lifecycle
install -m 755 "$featureDir/lifecycle/postCreate.sh" /opt/doghome/devcontainer/features/datadog-agent/lifecycle/postCreate.sh

# Fetch update-tool and use it to install ddtool. Ideally we'd like to use dotslash but it does not work well with ddtool auth helpers install because of shims.
curl --no-progress-meter --retry 10 --retry-max-time 60 -Lo /usr/local/bin/update-tool\
     https://binaries.ddbuild.io/devtools/bin/update-tool
chmod +x /usr/local/bin/update-tool

su - bits <<EOF
(umask 077 && mkdir -p ~/.local/state/workspaces)
touch ~/state-done
mkdir -p ~/.local/bin
touch ~/bin-done
update-tool ddtool@1.101.0 > ~/ddtool-install.log 2>&1
touch ~/ddtool-done

ddtool auth helpers install > ~/helpers-install.log 2>&1
touch ~/helpers-done

# Activate xdg-open functionality
ln -s workspaces-tool-helper ~/.local/bin/xdg-open
touch ~/xdg-open-done
EOF

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
EOF
