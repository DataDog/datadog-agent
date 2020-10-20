#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"

# Wait for any Running workflow
until [[ -z $(./argo list -l workflows.argoproj.io/phase=Running -o name) ]]; do
    sleep 10
done

if [[ -z $(./argo list -o name) ]]; then
    echo "No workflow found"
    exit 1
fi

set +x

for workflow in $(./argo list -l workflows.argoproj.io/phase=Succeeded -o name); do
    ./argo get "$workflow"
done

EXIT_CODE=0
for workflow in $(./argo list -l workflows.argoproj.io/phase=Failed -o name); do
    ./argo get "$workflow"
    EXIT_CODE=2
done

# Make the Argo UI available from the user
/opt/bin/kubectl patch svc -n argo argo-server --type json --patch $'[{"op": "replace", "path": "/spec/type", "value": "NodePort"}]'

until [[ -n ${KUBE_SVC:+x} ]]; do
    sleep 1
    KUBE_SVC="$(sudo iptables -w -t nat -L KUBE-NODEPORTS -n -v | awk '/argo\/argo-server:web/ && $3 ~ /^KUBE-SVC-/ {print $3}')"
done
sudo iptables -w -t nat -N HACK
sudo iptables -w -t nat -A HACK -m comment --comment 'argo/argo-server:web' -p tcp --dport 80 -j KUBE-MARK-MASQ
sudo iptables -w -t nat -A HACK -m comment --comment 'argo/argo-server:web' -p tcp --dport 80 -j "${KUBE_SVC}"
sudo iptables -w -t nat -A PREROUTING -m addrtype --dst-type LOCAL -j HACK
sudo iptables -w -t nat -A OUTPUT     -m addrtype --dst-type LOCAL -j HACK

TIME_LEFT=$(systemctl status terminate.timer | awk '$1 == "Trigger:" {print gensub(/ *Trigger: (.*)/, "\\1", 1)}')
LOCAL_IP=$(curl -s http://169.254.169.254/2020-10-27/meta-data/local-ipv4)

tput -T vt100 bold
echo "The Argo UI will remain available at http://${LOCAL_IP} until ${TIME_LEFT}"
tput -T vt100  sgr0

exit ${EXIT_CODE}
