#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"

# Wait for any Running workflow
until [[ "$(./argo list --running -o name)" == "No workflows found" ]]; do
    sleep 10
done

if [[ "$(./argo list -o name)" == "No workflows found" ]]; then
    echo "No workflow found"
    exit 1
fi

set +x

if ! locale -k LC_CTYPE | grep -qi 'charmap="utf-\+8"'; then
    no_utf8_opt='--no-utf8'
fi

for workflow in $(./argo list --status Succeeded -o name | grep -v 'No workflows found'); do
    ./argo get ${no_utf8_opt:-} "$workflow"
done

EXIT_CODE=0
for workflow in $(./argo list --status Failed -o name | grep -v 'No workflows found'); do
    ./argo get "$workflow" -o json | jq -r '.status.nodes[] | select(.phase == "Failed") | .displayName + " " + .id' | while read -r displayName podName; do
        printf '\033[1m===> Logs of %s\t%s <===\033[0m\n' "$displayName" "$podName"
        ./argo logs "$workflow" "$podName"
    done
    ./argo get ${no_utf8_opt:-} "$workflow"
    EXIT_CODE=2
done

# Make the Argo UI available from the user
/opt/bin/kubectl --namespace argo patch service/argo-server --type json --patch $'[{"op": "replace", "path": "/spec/type", "value": "NodePort"}]'

# The goal of the following iptables magic is to make the `argo-server` NodePort service available on port 80.
# We cannot do it with Kube since 80 isn’t in the NodePort service range and yet, 80 is a convenient port for an HTTP UI.
# It basically copies the iptables rules that are injected by Kuberntes for a NodePort service.
until [[ -n ${KUBE_SVC:+x} ]]; do
    sleep 1
    KUBE_SVC="$(sudo iptables -w -t nat -L KUBE-NODEPORTS -n -v | awk '/argo\/argo-server:web/ && $3 ~ /^KUBE-SVC-/ {print $3}')"
done
sudo iptables -w -t nat -N HACK
sudo iptables -w -t nat -A HACK -m comment --comment 'argo/argo-server:web' -p tcp --dport 80 -j KUBE-MARK-MASQ
sudo iptables -w -t nat -A HACK -m comment --comment 'argo/argo-server:web' -p tcp --dport 80 -j "${KUBE_SVC}"
sudo iptables -w -t nat -A PREROUTING -m addrtype --dst-type LOCAL -j HACK
sudo iptables -w -t nat -A OUTPUT     -m addrtype --dst-type LOCAL -j HACK

# In case of failure, let's keep the VM for 1 day instead of 2 hours for investigation
if [[ $EXIT_CODE != 0 ]]; then
    sudo sed -i 's/^OnBootSec=.*/OnBootSec=86400/' /etc/systemd/system/terminate.timer
    sudo systemctl daemon-reload
    sudo systemctl restart terminate.timer
fi

TIME_LEFT=$(systemctl status terminate.timer | awk '$1 == "Trigger:" {print gensub(/ *Trigger: (.*)/, "\\1", 1)}')
LOCAL_IP=$(curl -s http://169.254.169.254/2020-10-27/meta-data/local-ipv4)

printf "\033[1mThe Argo UI will remain available at \033[1;34mhttp://%s\033[0m until \033[1;33m%s\033[0m.\n" "$LOCAL_IP" "$TIME_LEFT"

exit ${EXIT_CODE}
