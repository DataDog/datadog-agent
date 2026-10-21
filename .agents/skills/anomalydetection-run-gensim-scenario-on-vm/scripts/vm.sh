#!/usr/bin/env bash
set -euo pipefail

SKILL_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SSH_PORT="${SSH_PORT:-22}"

fail() { echo "$*" >&2; exit 1; }
name() { [[ "$1" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || fail "Invalid name: $1"; }
target() {
    [[ "$VM" =~ ^[a-zA-Z0-9_.@:-]+$ && "$VM" != -* ]] || fail "Invalid SSH target"
    [[ "$SSH_PORT" =~ ^[1-9][0-9]*$ && "$SSH_PORT" -le 65535 ]] || fail "Invalid SSH port"
}
duration_seconds() {
    case "${1: -1}" in
        s) echo "${1%?}" ;;
        m) echo "$((${1%?} * 60))" ;;
        h) echo "$((${1%?} * 3600))" ;;
    esac
}
host() { ssh -p "$SSH_PORT" "$VM" bash -s -- "$@"; }
upload() { scp -P "$SSH_PORT" "$1" "$VM:$2"; }

remote() {
    {
        cat <<'EOF'
set -euo pipefail
export KUBECONFIG="$HOME/gensim/kubeconfig"
test -s "$KUBECONFIG"
test "$(kubectl config current-context)" = kind-gadget-dev
EOF
        cat
    } | host "$@"
}

create_vm() {
    name "$1"
    dda inv aws.create-vm "--stack-name=$1" "--instance-type=${2:-t3.2xlarge}" --no-install-agent
}

probe() {
    host <<'EOF'
set -euo pipefail
uname -m; nproc; free -h; df -h /
for tool in docker kind kubectl helm python3 curl; do command -v "$tool" || echo "missing: $tool"; done
sudo -n true
docker info >/dev/null
python3 -c 'import yaml'
sysctl fs.inotify.max_user_watches fs.inotify.max_user_instances
kind get clusters
EOF
}

bootstrap() {
    host <<'EOF'
set -euo pipefail
mkdir -p "$HOME/gensim"
EOF
    upload "$SKILL_ROOT/scripts/bootstrap.sh" gensim/bootstrap.sh
    host <<'EOF'
set -euo pipefail
bash "$HOME/gensim/bootstrap.sh" --disposable-vm
EOF
    ssh -p "$SSH_PORT" -O exit "$VM" || true
}

create_cluster() {
    host <<'EOF'
set -euo pipefail
clusters="$(kind get clusters)"
if grep -qx gadget-dev <<< "$clusters"; then echo "gadget-dev already exists; inspect and reuse it" >&2; exit 1; fi
mkdir -p "$HOME/gensim"
EOF
    upload "$SKILL_ROOT/assets/kind.yaml" gensim/kind.yaml
    host <<'EOF'
set -euo pipefail
kind create cluster --name gadget-dev --config "$HOME/gensim/kind.yaml" --kubeconfig "$HOME/gensim/kubeconfig"
EOF
    remote <<'EOF'
kubectl --context kind-gadget-dev wait --for=condition=Ready node --all --timeout=180s
EOF
}

reuse_cluster() {
    host <<'EOF'
set -euo pipefail
clusters="$(kind get clusters)"
grep -qx gadget-dev <<< "$clusters"
mkdir -p "$HOME/gensim"
kind export kubeconfig --name gadget-dev --kubeconfig "$HOME/gensim/kubeconfig"
EOF
    remote <<'EOF'
kubectl --context kind-gadget-dev wait --for=condition=Ready node --all --timeout=180s
EOF
}

deploy_recorder() {
    local env_file="$1" tag="$2" cluster_name="$3" flush="${4:-1m}" retention="${5:-90m}" recording_only="${6:-false}"
    [[ -f "$env_file" ]] || fail "Env file does not exist"
    [[ "$tag" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || fail "Invalid image tag"
    name "$cluster_name"
    [[ "$flush" =~ ^[1-9][0-9]*[smh]$ && "$retention" =~ ^[1-9][0-9]*[smh]$ ]] || fail "Use durations such as 1m or 90m"
    [[ "$(duration_seconds "$retention")" -gt "$(duration_seconds "$flush")" ]] || fail "Retention must exceed the flush interval"
    [[ "$recording_only" == true || "$recording_only" == false ]] || fail "recording-only must be true or false"
    remote <<'EOF'
chmod 700 "$HOME/gensim"
EOF
    upload "$env_file" gensim/env
    remote <<'EOF'
chmod 600 "$HOME/gensim/env"
EOF
    local values
    values="$(mktemp)"
    sed -e "s/__TAG__/$tag/g" -e "s/__CLUSTER_NAME__/$cluster_name/g" -e "s/__FLUSH__/$flush/g" \
        -e "s/__RETENTION__/$retention/g" -e "s/__RECORDING_ONLY__/$recording_only/g" \
        "$SKILL_ROOT/assets/recorder-values.yaml" > "$values"
    upload "$values" gensim/recorder-values.yaml
    rm -- "$values"
    remote "$tag" <<'EOF'
set -a; . "$HOME/gensim/env"; set +a
test -n "${DD_API_KEY:-}"; test -n "${DD_APP_KEY:-}"
kubectl --context kind-gadget-dev --namespace default create secret generic datadog-secret \
    --from-file=api-key=<(printf %s "$DD_API_KEY") --dry-run=client -o yaml |
    kubectl --context kind-gadget-dev --namespace default apply -f -
docker pull --platform linux/amd64 "datadog/agent-dev:$1"
docker image save --platform linux/amd64 -o "$HOME/gensim/agent.tar" "datadog/agent-dev:$1"
kind load image-archive "$HOME/gensim/agent.tar" --name gadget-dev
rm -- "$HOME/gensim/agent.tar"
helm repo add datadog https://helm.datadoghq.com --force-update
helm repo update
helm --kube-context kind-gadget-dev --namespace default upgrade --install datadog-agent -f "$HOME/gensim/recorder-values.yaml" datadog/datadog
kubectl --context kind-gadget-dev --namespace default rollout status daemonset/datadog-agent --timeout=180s
EOF
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    command="${1:?Usage: vm.sh command --disposable-vm VM [arguments]}"
    shift
    if [[ "$command" == create-vm ]]; then
        [[ "${1:-}" == --disposable-vm ]] || fail "Requires --disposable-vm"
        shift
        create_vm "$@"
        exit
    fi
    if [[ "$command" != probe ]]; then
        [[ "${1:-}" == --disposable-vm ]] || fail "Requires --disposable-vm"
        shift
    fi
    VM="${1:?Supply an SSH target}"
    shift
    target
    case "$command" in
        probe) probe ;;
        bootstrap) bootstrap ;;
        create-cluster) create_cluster ;;
        reuse-cluster) reuse_cluster ;;
        deploy-recorder) deploy_recorder "$@" ;;
        *) fail "Unknown command: $command" ;;
    esac
fi
