#!/bin/bash
# Blocks until every IBM MQ queue manager is Running and its listener port accepts
# connections, dumping diagnostics on failure so an opaque create hang becomes a
# legible root cause. Runs as root via `sudo bash -s`.
set -euo pipefail

: "${MQ_NQMGRS:?}" "${MQ_BASE_PORT:?}" "${MQ_QM_PREFIX:?}"

MQ_HOME=/opt/mqm
export PATH="${MQ_HOME}/bin:${PATH}"

diag() {
  local qm="$1" port="$2"
  echo "[mq_readiness] FAILED for ${qm} (port ${port}) — diagnostics:" >&2
  runuser -u mqm -- dspmq >&2 2>&1 || true
  runuser -u mqm -- dspmq -m "${qm}" -o status >&2 2>&1 || true
  ss -ltnp >&2 2>&1 || true
  systemctl status ibm-mq-load.service --no-pager >&2 2>&1 || true
  journalctl -u ibm-mq-load.service -n 100 --no-pager >&2 2>&1 || true
  find "${MQ_HOME}/bin" -maxdepth 1 >&2 2>&1 || true
}

for i in $(seq 1 "${MQ_NQMGRS}"); do
  QM="${MQ_QM_PREFIX}${i}"
  PORT=$(( MQ_BASE_PORT + i - 1 ))
  ok=0
  for _ in $(seq 1 60); do
    if runuser -u mqm -- dspmq -m "${QM}" 2>/dev/null | grep -q "STATUS(Running)" \
       && (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") 2>/dev/null; then
      ok=1
      echo "[mq_readiness] ${QM} Running and listening on ${PORT}"
      break
    fi
    sleep 5
  done
  if [ "${ok}" -ne 1 ]; then
    diag "${QM}" "${PORT}"
    exit 1
  fi
done

echo "[mq_readiness] all ${MQ_NQMGRS} queue managers ready"
