#!/bin/bash
# Autonomous gensim episode runner - runs on EC2 VM
set -euo pipefail

LOG_FILE="/tmp/gensim-runner.log"
exec > >(tee -a "$LOG_FILE") 2>&1

echo "[$(date -u)] Starting gensim runner for episode: ${EPISODE_NAME}"

# Source secrets (DD_API_KEY, DD_APP_KEY)
# shellcheck source=/dev/null
source /tmp/gensim-secrets.env

# Build kubeconfig from local Kind cluster
kind get kubeconfig --name "${CLUSTER_NAME}" > /tmp/kubeconfig
export KUBECONFIG=/tmp/kubeconfig
echo "[$(date -u)] Kubeconfig ready"

# Run play-episode.sh, capturing output to a dedicated log
# (file is root-owned after Pulumi copy, so we invoke via bash rather than relying on the execute bit)
cd /tmp/gensim-episode
bash ./play-episode.sh run-episode "${SCENARIO}" > /tmp/play-episode.log 2>&1
cat /tmp/play-episode.log
echo "[$(date -u)] Episode completed"

# Collect parquet files from the Datadog Agent pod.
# kubectl is not installed on the VM; use the Kind control-plane container which has it.
KUBE="docker exec ${CLUSTER_NAME}-control-plane kubectl"
POD=$(${KUBE} get pod -n "${KUBE_NAMESPACE}" -l app.kubernetes.io/component=agent \
      -o jsonpath='{.items[0].metadata.name}')
echo "[$(date -u)] Collecting parquet files from pod ${POD}"
# kubectl cp fails because /tmp in the control-plane container is a tmpfs mount
# and docker cp cannot read through tmpfs (it uses the overlay filesystem).
# Instead: pipe tar from the pod via kubectl exec into /root of the control-plane
# (which is on the overlay fs), then docker cp to the VM.
${KUBE} exec -n "${KUBE_NAMESPACE}" "${POD}" -c agent -- tar cf - -C /tmp observer-metrics \
    | docker exec -i "${CLUSTER_NAME}-control-plane" tar xf - -C /root
mkdir -p /tmp/gensim-parquet
docker cp "${CLUSTER_NAME}-control-plane:/root/observer-metrics" /tmp/gensim-parquet/
echo "[$(date -u)] Parquet files collected from pod ${POD}"

# Archive: parquet + play-episode log
mkdir -p /tmp/gensim-archive
cp -r /tmp/gensim-parquet          /tmp/gensim-archive/parquet/
cp /tmp/play-episode.log           /tmp/gensim-archive/
cp "$LOG_FILE"                     /tmp/gensim-archive/gensim-runner.log || true
if [ -d /tmp/gensim-episode/results ]; then cp -r /tmp/gensim-episode/results /tmp/gensim-archive/; fi

ARCHIVE="/tmp/gensim-results-${EPISODE_NAME}-$(date -u +%Y%m%d).zip"
zip -r "${ARCHIVE}" /tmp/gensim-archive/
echo "[$(date -u)] Archive created: ${ARCHIVE}"

# Upload to S3 (only if bucket is set)
if [ -n "${S3_BUCKET:-}" ]; then
    S3_DEST="s3://${S3_BUCKET}/$(basename "${ARCHIVE}")"
    aws s3 cp "${ARCHIVE}" "${S3_DEST}"
    echo "[$(date -u)] Uploaded to ${S3_DEST}"
fi

echo "[$(date -u)] All done."
