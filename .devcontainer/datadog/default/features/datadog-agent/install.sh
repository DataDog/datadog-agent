#!/bin/bash


# Get claude from the buildimages /root/.local/bin
cp /root/.local/bin/claude /home/bits/.local/bin/claude

# Configure PATH for interactive shells.
# File name convention *-workspace-env.sh is important:
# /etc/zsh/zshenv sources these files.
cat > /etc/profile.d/10-ddagent-workspace-env.sh << 'EOF'
export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
EOF
