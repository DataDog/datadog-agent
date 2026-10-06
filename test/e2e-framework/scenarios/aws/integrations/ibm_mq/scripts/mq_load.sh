#!/bin/bash
# Installs a continuous IBM MQ put/get load generator as a systemd service so queue
# depth and enqueue/dequeue rate metric families are non-zero (PCF cost driver).
# Runs as root via `sudo bash -s`. MQ_* variables are prepended by the scenario.
set -euo pipefail

: "${MQ_NQMGRS:?}" "${MQ_QUEUES_PER_QM:?}" "${MQ_QM_PREFIX:?}" "${MQ_QUEUE_PREFIX:?}"

MQ_HOME=/opt/mqm
SAMP="${MQ_HOME}/samp/bin"
LOADER=/usr/local/bin/ibm-mq-load.sh

# The loop uses the MQ sample programs amqsput/amqsget (installed by
# MQSeriesSamples) to put a bounded-random batch of messages and drain a fraction,
# rotating across queue managers and queues so depth stays busy but bounded.
cat >"${LOADER}" <<LOADER_EOF
#!/bin/bash
set -u
export PATH="${SAMP}:${MQ_HOME}/bin:\$PATH"
NQMGRS=${MQ_NQMGRS}
QUEUES_PER_QM=${MQ_QUEUES_PER_QM}
QM_PREFIX="${MQ_QM_PREFIX}"
QUEUE_PREFIX="${MQ_QUEUE_PREFIX}"

while true; do
  qm_idx=\$(( (RANDOM % NQMGRS) + 1 ))
  q_idx=\$(( (RANDOM % QUEUES_PER_QM) + 1 ))
  QM="\${QM_PREFIX}\${qm_idx}"
  Q="\${QUEUE_PREFIX}\${q_idx}"

  # Put a bounded-random batch (5..25 messages).
  n=\$(( (RANDOM % 21) + 5 ))
  { for m in \$(seq 1 \$n); do echo "load message \$m \$(date +%s%N)"; done; echo; } \
    | amqsput "\$Q" "\$QM" >/dev/null 2>&1 || true

  # Drain a fraction so depth churns but does not drain to zero (amqsget exits
  # after its wait interval when the queue is empty).
  if [ \$(( RANDOM % 2 )) -eq 0 ]; then
    timeout 5 amqsget "\$Q" "\$QM" >/dev/null 2>&1 || true
  fi

  sleep \$(( (RANDOM % 3) + 1 ))
done
LOADER_EOF
chmod +x "${LOADER}"

cat >/etc/systemd/system/ibm-mq-load.service <<UNIT_EOF
[Unit]
Description=IBM MQ continuous put/get load generator
After=network.target

[Service]
Type=simple
User=mqm
Group=mqm
ExecStart=${LOADER}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT_EOF

systemctl daemon-reload
systemctl enable --now ibm-mq-load.service
echo "[mq_load] load generator started across ${MQ_NQMGRS} queue managers"
