#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == --disposable-vm ]] || { echo "Requires --disposable-vm" >&2; exit 1; }
grep -Eq '^ID="?ubuntu"?$' /etc/os-release
[[ "$(uname -m)" == x86_64 ]] || { echo "Requires Ubuntu x86_64" >&2; exit 1; }
sudo -n true
sudo apt-get update -qq
sudo apt-get install -y -qq curl ca-certificates gpg python3-yaml
sudo install -m 0755 -d /etc/apt/keyrings

if ! command -v docker >/dev/null; then
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
    sudo apt-get update -qq
    sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    sudo systemctl enable --now docker
fi
sudo usermod -aG docker "$USER"

if ! command -v kubectl >/dev/null; then
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.32/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.32/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
    sudo apt-get update -qq
    sudo apt-get install -y -qq kubectl
fi
if ! command -v kind >/dev/null; then
    sudo curl -fsSLo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.27.0/kind-linux-amd64
    sudo chmod +x /usr/local/bin/kind
fi
if ! command -v helm >/dev/null; then
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | sudo DESIRED_VERSION=v3.20.2 bash
fi
printf 'fs.inotify.max_user_watches=524288\nfs.inotify.max_user_instances=1024\n' | sudo tee /etc/sysctl.d/99-kind.conf >/dev/null
sudo sysctl -p /etc/sysctl.d/99-kind.conf
