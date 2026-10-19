#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == --disposable-vm ]] || { echo "Requires --disposable-vm" >&2; exit 1; }
grep -Eq '^ID="?ubuntu"?$' /etc/os-release
[[ "$(uname -m)" == x86_64 ]] || { echo "Requires Ubuntu x86_64" >&2; exit 1; }
sudo -n true
sudo apt-get update -qq
sudo apt-get install -y -qq curl ca-certificates gpg python3-yaml
sudo install -m 0755 -d /etc/apt/keyrings

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"

curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.32/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.32/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y -qq kubectl
if ! command -v kind >/dev/null; then
    if command -v go >/dev/null; then
        mkdir -p "$HOME/.local/bin"
        GOBIN="$HOME/.local/bin" go install sigs.k8s.io/kind@v0.27.0
        sudo install -m 0755 "$HOME/.local/bin/kind" /usr/local/bin/kind
    else
        sudo curl -fsSLo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.27.0/kind-linux-amd64
        sudo chmod +x /usr/local/bin/kind
    fi
fi
helm_key="$(mktemp)"
trap 'rm -f -- "$helm_key"' EXIT
curl -fsSL https://packages.buildkite.com/helm-linux/helm-debian/gpgkey > "$helm_key"
fingerprint="$(gpg --show-keys --with-colons "$helm_key" | awk -F: '$1 == "fpr" && !seen++ {print $10}')"
[[ "$fingerprint" == DDF78C3E6EBB2D2CC223C95C62BA89D07698DBC6 ]] || { echo "Unexpected Helm APT signing key" >&2; exit 1; }
sudo install -m 0644 "$helm_key" /etc/apt/keyrings/helm.asc
sudo tee /etc/apt/sources.list.d/helm-stable-debian.sources >/dev/null <<'EOF'
Types: deb
URIs: https://packages.buildkite.com/helm-linux/helm-debian/any/
Suites: any
Components: main
Signed-By: /etc/apt/keyrings/helm.asc
EOF
sudo apt-get update -qq
sudo apt-get install -y -qq helm=3.20.2-1
printf 'fs.inotify.max_user_watches=524288\nfs.inotify.max_user_instances=1024\n' | sudo tee /etc/sysctl.d/99-kind.conf >/dev/null
sudo sysctl -p /etc/sysctl.d/99-kind.conf
