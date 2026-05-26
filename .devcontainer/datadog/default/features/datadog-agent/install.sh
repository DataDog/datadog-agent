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


su - bits <<EOF
export PATH="~/.local/bin:\$PATH"
(umask 077 && mkdir -p ~/.local/state/workspaces)
touch ~/state-done


ddtool auth helpers install > ~/helpers-install.log 2>&1
touch ~/helpers-done

# Activate xdg-open functionality
ln -s /usr/local/bin/workspaces-tool-helper ~/.local/bin/xdg-open
touch ~/xdg-open-done
EOF

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"
export PASSWORD_STORE_GPG_OPTS="--homedir $HOME/.config/password-store/gpg"
EOF
