#!/usr/bin/env bash
# Build the custom dd-agent image (linux/amd64): patched ./cmd/agent + scrappy assets.
# Prereqs: dda dev container running (dda env dev start) + colima docker + gcloud SA.
set -euo pipefail
TAG="${1:?usage: ./build-agent-image.sh <gar-tag>   e.g. scrappy-detect-$(date +%Y%m%d)-test}"
REG=us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images/agent-dev
GEN="$(cd "$(dirname "$0")" && pwd)"; A="$GEN/scrappy-assets"
[ -f "$A/model.scrappy" ] || "$A/fetch-model.sh"
echo "== cross-compiling ./cmd/agent (linux/amd64) in dev container =="
docker --context colima exec dda-linux-container-default bash -lc '
  set -e; cd /root/repos/datadog-agent
  git config --global --add safe.directory /root/repos/datadog-agent 2>/dev/null || true
  export CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-gnu-gcc CXX=x86_64-linux-gnu-g++
  TAGS=$(dda inv print-default-build-tags --build agent --platform linux | tr "," "\n" | grep -vx python | paste -sd, -)
  echo "build tags: $TAGS"
  go build -tags "$TAGS" -o /tmp/agent-amd64 ./cmd/agent'
docker --context colima cp dda-linux-container-default:/tmp/agent-amd64 "$A/agent"
cat > "$A/Dockerfile" <<'EOF'
FROM datadog/agent:7
COPY agent         /opt/datadog-agent/bin/agent/agent
COPY scrappy-infer /opt/scrappy/scrappy-infer
COPY model.scrappy /opt/scrappy/model.scrappy
COPY vocab.json    /opt/scrappy/vocab.json
RUN chmod +x /opt/datadog-agent/bin/agent/agent /opt/scrappy/scrappy-infer
EOF
: "${CLOUDSDK_CORE_ACCOUNT:=gensim-integration@dd-plt-simulation-environment.iam.gserviceaccount.com}"; export CLOUDSDK_CORE_ACCOUNT
echo "== building + pushing $REG:$TAG =="
docker --context colima buildx build --builder colima --platform linux/amd64 -t "$REG:$TAG" --push "$A"
echo "pushed $REG:$TAG"
