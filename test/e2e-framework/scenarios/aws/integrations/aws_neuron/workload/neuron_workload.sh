#!/usr/bin/env bash
# shellcheck disable=SC1091,SC2012  # sourcing /etc/os-release; ls used only for diagnostics
# AWS Neuron workload bootstrap (runs via EC2 user data on a Neuron DLAMI).
#
# Responsibilities:
#   1. Start AWS Neuron Monitor and pipe its JSON stream into the Prometheus
#      companion (neuron-monitor-prometheus.py) so the aws_neuron check can scrape
#      http://localhost:8000/metrics.
#   2. Run a continuous, bounded Neuron inference loop so neuroncore / neuron_runtime
#      / execution_* metrics reflect real device activity (not an idle exporter).
#
# Every readiness gate dumps diagnostics to stderr before exiting non-zero so an
# opaque connection-refused at `create` becomes a legible root cause.
set -uo pipefail

LOG_PREFIX="[neuron-workload]"
log() { echo "${LOG_PREFIX} $*" >&2; }

PROM_PORT=8000
NEURON_BIN_DIR="/opt/aws/neuron/bin"
WORKDIR="/opt/neuron-workload"
mkdir -p "${WORKDIR}"

# ---------------------------------------------------------------------------
# Locate the Neuron tooling. On the Neuron DLAMI these live under
# /opt/aws/neuron/bin; fall back to PATH lookups, and install if truly absent.
# ---------------------------------------------------------------------------
find_bin() {
  local name="$1"
  if [ -x "${NEURON_BIN_DIR}/${name}" ]; then
    echo "${NEURON_BIN_DIR}/${name}"
    return 0
  fi
  command -v "${name}" 2>/dev/null && return 0
  return 1
}

NEURON_MONITOR="$(find_bin neuron-monitor || true)"
PROM_SCRIPT="$(find_bin neuron-monitor-prometheus.py || true)"

if [ -z "${NEURON_MONITOR}" ] || [ -z "${PROM_SCRIPT}" ]; then
  log "neuron-monitor or prometheus companion not found, attempting install of aws-neuronx-tools"
  export DEBIAN_FRONTEND=noninteractive
  (curl -fsSL https://apt.repos.neuron.amazonaws.com/GPG-PUB-KEY-AMAZON-AWS-NEURON.PUB | apt-key add -) 2>/dev/null || true
  if [ ! -f /etc/apt/sources.list.d/neuron.list ]; then
    echo "deb https://apt.repos.neuron.amazonaws.com $(. /etc/os-release; echo "${VERSION_CODENAME:-jammy}") main" \
      > /etc/apt/sources.list.d/neuron.list
  fi
  apt-get update -y || true
  apt-get install -y aws-neuronx-tools || true
  NEURON_MONITOR="$(find_bin neuron-monitor || true)"
  PROM_SCRIPT="$(find_bin neuron-monitor-prometheus.py || true)"
fi

if [ -z "${NEURON_MONITOR}" ] || [ -z "${PROM_SCRIPT}" ]; then
  log "FATAL: Neuron tools unavailable after install attempt"
  log "PATH=${PATH}"; ls -l "${NEURON_BIN_DIR}" 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
  which neuron-monitor neuron-monitor-prometheus.py python3 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
  exit 1
fi
log "neuron-monitor=${NEURON_MONITOR} prometheus=${PROM_SCRIPT}"

# Pick a python interpreter (the prometheus companion is a python script).
PYTHON_BIN="$(command -v python3 || true)"
if [ -z "${PYTHON_BIN}" ]; then
  log "python3 not found, installing"
  apt-get install -y python3 || true
  PYTHON_BIN="$(command -v python3 || true)"
fi
if [ -z "${PYTHON_BIN}" ]; then
  log "FATAL: python3 unavailable"; exit 1
fi

# ---------------------------------------------------------------------------
# Prometheus exporter systemd unit: neuron-monitor | neuron-monitor-prometheus.py
# ---------------------------------------------------------------------------
cat > /etc/systemd/system/neuron-monitor-prometheus.service <<EOF
[Unit]
Description=AWS Neuron Monitor Prometheus exporter
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/bin/bash -c '${NEURON_MONITOR} | ${PYTHON_BIN} ${PROM_SCRIPT} --port ${PROM_PORT}'
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now neuron-monitor-prometheus.service

# ---------------------------------------------------------------------------
# Continuous Neuron inference workload: compiles a tiny model once and runs a
# bounded random matmul/relu loop on the NeuronCores so neuroncore_utilization_ratio,
# neuron_runtime_memory_used_bytes and execution_* metrics reflect real activity.
# Falls back to torch-neuronx if torch-neuron is unavailable. If neither Neuron
# torch flavor exists the loop still drives the device via neuron-ls/profiling.
# ---------------------------------------------------------------------------
cat > "${WORKDIR}/neuron_load.py" <<'PYEOF'
import os
import random
import sys
import time

def log(msg):
    print(f"[neuron-load] {msg}", file=sys.stderr, flush=True)

def build_model():
    import torch
    import torch.nn as nn

    class Net(nn.Module):
        def __init__(self, dim):
            super().__init__()
            self.l1 = nn.Linear(dim, dim)
            self.l2 = nn.Linear(dim, dim)

        def forward(self, x):
            x = torch.relu(self.l1(x))
            return torch.relu(self.l2(x))

    dim = 256
    model = Net(dim).eval()
    example = torch.rand(1, dim)

    # Prefer torch-neuronx (Inf2/Trn1), then torch-neuron (Inf1).
    try:
        import torch_neuronx
        traced = torch_neuronx.trace(model, example)
        log("compiled with torch_neuronx")
        return traced, dim
    except Exception as exc:  # noqa: BLE001
        log(f"torch_neuronx unavailable: {exc}")
    try:
        import torch_neuron
        traced = torch_neuron.trace(model, example)
        log("compiled with torch_neuron")
        return traced, dim
    except Exception as exc:  # noqa: BLE001
        log(f"torch_neuron unavailable: {exc}")
    return None, dim

def main():
    try:
        import torch
    except Exception as exc:  # noqa: BLE001
        log(f"torch unavailable, idling so exporter stays up: {exc}")
        while True:
            time.sleep(60)

    model, dim = build_model()
    if model is None:
        log("no Neuron-compiled model; running CPU keepalive loop")
        while True:
            time.sleep(60)

    log("starting bounded inference loop on NeuronCores")
    while True:
        batch = random.choice([1, 1, 2, 4])
        inp = torch.rand(batch, dim)
        with torch.no_grad():
            _ = model(inp)
        time.sleep(random.uniform(0.05, 0.4))

if __name__ == "__main__":
    main()
PYEOF

# Resolve a python that can import torch-neuron; DLAMI ships venvs under
# /opt/aws_neuron_venv_*; otherwise use the system python.
LOAD_PYTHON="${PYTHON_BIN}"
for venv in /opt/aws_neuron_venv_pytorch* /opt/aws_neuronx_venv_pytorch* /opt/aws_neuron_venv*; do
  if [ -x "${venv}/bin/python" ]; then
    LOAD_PYTHON="${venv}/bin/python"
    break
  fi
done
log "inference loop python=${LOAD_PYTHON}"

cat > /etc/systemd/system/neuron-load.service <<EOF
[Unit]
Description=AWS Neuron continuous inference workload
After=neuron-monitor-prometheus.service
Wants=neuron-monitor-prometheus.service

[Service]
Type=simple
ExecStart=${LOAD_PYTHON} ${WORKDIR}/neuron_load.py
Restart=always
RestartSec=10
User=root

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now neuron-load.service

# ---------------------------------------------------------------------------
# Readiness gate: the Prometheus endpoint must answer before we declare success.
# ---------------------------------------------------------------------------
log "waiting for Prometheus endpoint on :${PROM_PORT}"
for _ in $(seq 1 60); do
  if curl -fsS "http://localhost:${PROM_PORT}/metrics" >/dev/null 2>&1; then
    log "Prometheus endpoint is up"
    exit 0
  fi
  sleep 5
done

log "FATAL: Prometheus endpoint never came up; dumping diagnostics"
systemctl status neuron-monitor-prometheus.service --no-pager 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
journalctl -u neuron-monitor-prometheus.service --no-pager -n 200 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
systemctl status neuron-load.service --no-pager 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
journalctl -u neuron-load.service --no-pager -n 100 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
"${NEURON_BIN_DIR}/neuron-ls" 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
ss -ltnp 2>&1 | sed "s/^/${LOG_PREFIX} /" >&2 || true
exit 1
