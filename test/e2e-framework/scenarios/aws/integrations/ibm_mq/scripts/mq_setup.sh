#!/bin/bash
# IBM MQ Advanced for Developers 9.3 install + queue-manager provisioning.
# Runs as root via `sudo bash -s`. MQ_* variables are prepended by the scenario.
# Phase-1 skeleton: hardened during live exploration/synthesis.
set -euo pipefail

: "${MQ_NQMGRS:?}" "${MQ_QUEUES_PER_QM:?}" "${MQ_BASE_PORT:?}"
: "${MQ_DOWNLOAD_URL:?}" "${MQ_CHANNEL:?}" "${MQ_QM_PREFIX:?}" "${MQ_QUEUE_PREFIX:?}"

log() { echo "[mq_setup] $*"; }
diag() {
  echo "[mq_setup] FAILED — diagnostics:" >&2
  { command -v dspmqver >/dev/null 2>&1 && dspmqver; } >&2 2>&1 || true
  find /opt/mqm/bin -maxdepth 1 2>&1 | head -n 20 >&2 || true
  runuser -u mqm -- dspmq >&2 2>&1 || true
  rpm -qa 'MQSeries*' >&2 2>&1 || true
  journalctl -n 100 --no-pager >&2 2>&1 || true
}
trap diag ERR

MQ_HOME=/opt/mqm
INSTALL_TMP=/tmp/mq-install

if [ ! -x "${MQ_HOME}/bin/crtmqm" ]; then
  log "installing IBM MQ ${MQ_VERSION:-9.3} from ${MQ_DOWNLOAD_URL}"
  # Deps: download tooling + libraries MQ RPMs expect on EL9.
  dnf install -y --allowerasing tar gzip wget procps-ng iproute glibc shadow-utils >/dev/null 2>&1 || \
    yum install -y tar gzip wget procps-ng iproute glibc shadow-utils >/dev/null 2>&1 || true

  rm -rf "${INSTALL_TMP}"
  mkdir -p "${INSTALL_TMP}"
  wget -q -O "${INSTALL_TMP}/mqadv.tar.gz" "${MQ_DOWNLOAD_URL}"
  tar -xzf "${INSTALL_TMP}/mqadv.tar.gz" -C "${INSTALL_TMP}"

  # The tar unpacks into a MQServer directory containing mqlicense.sh + RPMs.
  MQ_SRC="$(dirname "$(find "${INSTALL_TMP}" -name mqlicense.sh | head -n1)")"
  [ -n "${MQ_SRC}" ] || { echo "mqlicense.sh not found in tarball" >&2; exit 1; }
  log "accepting developer license (LAP) under ${MQ_SRC}"
  ( cd "${MQ_SRC}" && MQLICENSE=accept ./mqlicense.sh -accept -text_only )

  # Core developer package set (runtime + server + samples + client + SDK + GSKit + JRE).
  log "installing MQ RPMs"
  ( cd "${MQ_SRC}" && rpm -ivh --force \
      MQSeriesRuntime-*.rpm \
      MQSeriesServer-*.rpm \
      MQSeriesSamples-*.rpm \
      MQSeriesClient-*.rpm \
      MQSeriesSDK-*.rpm \
      MQSeriesGSKit-*.rpm \
      MQSeriesJRE-*.rpm )

  "${MQ_HOME}/bin/setmqinst" -i -p "${MQ_HOME}"
else
  log "IBM MQ already installed at ${MQ_HOME}"
fi

# Wire the MQ client libraries for the Datadog Agent (pymqi) via ldconfig so the
# agent systemd service resolves them without a per-service LD_LIBRARY_PATH.
echo "${MQ_HOME}/lib64" >/etc/ld.so.conf.d/mqm64.conf
echo "${MQ_HOME}/lib"  >>/etc/ld.so.conf.d/mqm64.conf
ldconfig

# Create N queue managers, each with its own listener port, DEV.ADMIN.SVRCONN
# channel (dev auth disabled so the check connects with no credentials), and
# QueuesPerQM local queues.
for i in $(seq 1 "${MQ_NQMGRS}"); do
  QM="${MQ_QM_PREFIX}${i}"
  PORT=$(( MQ_BASE_PORT + i - 1 ))

  if runuser -u mqm -- dspmq -m "${QM}" 2>/dev/null | grep -q "${QM}"; then
    log "queue manager ${QM} already exists"
  else
    log "creating queue manager ${QM} (listener port ${PORT})"
    runuser -u mqm -- crtmqm "${QM}"
  fi
  runuser -u mqm -- strmqm "${QM}" || true

  MQSC="/tmp/${QM}.mqsc"
  {
    echo "DEFINE LISTENER(LISTENER.TCP) TRPTYPE(TCP) PORT(${PORT}) CONTROL(QMGR) REPLACE"
    echo "DEFINE CHANNEL(${MQ_CHANNEL}) CHLTYPE(SVRCONN) MCAUSER('mqm') REPLACE"
    # Developer-friendly auth: disable CHLAUTH + CONNAUTH so the check connects
    # anonymously. Exploration will optionally tighten this to user/password.
    echo "ALTER QMGR CHLAUTH(DISABLED)"
    echo "ALTER QMGR CONNAUTH(' ')"
    echo "REFRESH SECURITY TYPE(CONNAUTH)"
    for q in $(seq 1 "${MQ_QUEUES_PER_QM}"); do
      echo "DEFINE QLOCAL(${MQ_QUEUE_PREFIX}${q}) DEFPSIST(YES) MAXDEPTH(50000) REPLACE"
    done
  } >"${MQSC}"
  chown mqm:mqm "${MQSC}"
  runuser -u mqm -- runmqsc "${QM}" <"${MQSC}"

  # Start the listener now. CONTROL(QMGR) only auto-starts it at queue-manager
  # startup, and the QM was already running when the listener was defined above,
  # so it must be started explicitly on first provision. Run it as a separate,
  # failure-tolerant command: on a re-run the listener is already active and
  # runmqsc returns AMQ8730W (non-zero), which would otherwise trip `set -e`.
  runuser -u mqm -- runmqsc "${QM}" <<<"START LISTENER(LISTENER.TCP)" || true
  log "queue manager ${QM} configured with ${MQ_QUEUES_PER_QM} queues"
done

log "IBM MQ setup complete: ${MQ_NQMGRS} queue managers, $(( MQ_NQMGRS * MQ_QUEUES_PER_QM )) total queues"
