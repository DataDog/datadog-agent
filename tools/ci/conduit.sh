#!/usr/bin/env bash
# Download, start, and stop bazel-conduit for a CI Bazel job.
# start: fetch the pinned Linux x86_64 release, serve BES on 127.0.0.1:8080,
#        export OTLP to the node Agent at http://$BUILDENV_HOST_IP:4317.
# stop:  SIGTERM the server and wait for the exporter flush.
set -euo pipefail

CONDUIT_VERSION="${CONDUIT_VERSION:-0.0.1-RC1}"
# sha256 of conduit-linux-x86_64 from the v0.0.1-RC1 GitHub release.
CONDUIT_SHA256="${CONDUIT_SHA256:-a0c08a2524a9cb0b3c8d4aa7754788368d9f979acb207518bb80c1c19e8b323e}"
CONDUIT_ASSET="conduit-linux-x86_64"
CONDUIT_URL="${CONDUIT_URL:-https://github.com/JSGette/bazel_conduit/releases/download/v${CONDUIT_VERSION}/${CONDUIT_ASSET}}"
CONDUIT_DIR="${CONDUIT_DIR:-${CI_PROJECT_DIR:?CI_PROJECT_DIR is required}/.conduit}"
CONDUIT_BIN="${CONDUIT_DIR}/${CONDUIT_ASSET}"
CONDUIT_PID_FILE="${CONDUIT_DIR}/conduit.pid"
CONDUIT_LOG="${CONDUIT_DIR}/conduit.log"
# Must match the bes_backend port written to user.bazelrc by the coverage job.
CONDUIT_PORT=8080
CONDUIT_LISTEN_TIMEOUT_S="${CONDUIT_LISTEN_TIMEOUT_S:-5}"
CONDUIT_STOP_TIMEOUT_S="${CONDUIT_STOP_TIMEOUT_S:-15}"

usage() {
    echo "usage: $0 start|stop" >&2
    exit 2
}

wait_for_port() {
    local deadline=$((SECONDS + CONDUIT_LISTEN_TIMEOUT_S))
    while ((SECONDS < deadline)); do
        if bash -c "echo >/dev/tcp/127.0.0.1/${CONDUIT_PORT}" 2>/dev/null; then
            return 0
        fi
        sleep 0.1
    done
    return 1
}

start() {
    if [[ -z "${BUILDENV_HOST_IP:-}" ]]; then
        echo "BUILDENV_HOST_IP is unset; cannot export OTLP to the node Agent" >&2
        exit 1
    fi

    mkdir -p "${CONDUIT_DIR}"
    # GitHub release fetches flake on transient network/5xx; retry before aborting coverage.
    "${CI_PROJECT_DIR}/tools/ci/retry.sh" -n 5 curl -fsSL -o "${CONDUIT_BIN}" "${CONDUIT_URL}"
    echo "${CONDUIT_SHA256}  ${CONDUIT_BIN}" | sha256sum -c -
    chmod +x "${CONDUIT_BIN}"

    "${CONDUIT_BIN}" --version

    local otlp_endpoint="http://${BUILDENV_HOST_IP}:4317"
    : >"${CONDUIT_LOG}"
    nohup "${CONDUIT_BIN}" serve \
        --export otlp \
        --otlp-endpoint "${otlp_endpoint}" \
        --port "${CONDUIT_PORT}" \
        >>"${CONDUIT_LOG}" 2>&1 &
    echo $! >"${CONDUIT_PID_FILE}"

    if ! wait_for_port; then
        echo "conduit did not listen on 127.0.0.1:${CONDUIT_PORT} within ${CONDUIT_LISTEN_TIMEOUT_S}s" >&2
        if [[ -f "${CONDUIT_LOG}" ]]; then
            cat "${CONDUIT_LOG}" >&2 || true
        fi
        exit 1
    fi

    echo "conduit listening on 127.0.0.1:${CONDUIT_PORT}, OTLP -> ${otlp_endpoint}"
}

stop() {
    if [[ ! -f "${CONDUIT_PID_FILE}" ]]; then
        echo "conduit pid file missing; nothing to stop"
        return 0
    fi

    local pid
    pid="$(cat "${CONDUIT_PID_FILE}")"
    if ! kill -0 "${pid}" 2>/dev/null; then
        echo "conduit pid ${pid} is not running"
        rm -f "${CONDUIT_PID_FILE}"
        if [[ -f "${CONDUIT_LOG}" ]]; then
            cat "${CONDUIT_LOG}" || true
        fi
        return 0
    fi

    kill -TERM "${pid}" 2>/dev/null || true
    local deadline=$((SECONDS + CONDUIT_STOP_TIMEOUT_S))
    while kill -0 "${pid}" 2>/dev/null && ((SECONDS < deadline)); do
        sleep 0.2
    done
    if kill -0 "${pid}" 2>/dev/null; then
        echo "conduit pid ${pid} still running after ${CONDUIT_STOP_TIMEOUT_S}s; sending SIGKILL" >&2
        kill -KILL "${pid}" 2>/dev/null || true
    fi
    wait "${pid}" 2>/dev/null || true
    rm -f "${CONDUIT_PID_FILE}"

    if [[ -f "${CONDUIT_LOG}" ]]; then
        echo "----- conduit.log -----"
        cat "${CONDUIT_LOG}" || true
        echo "----- end conduit.log -----"
    fi
}

case "${1:-}" in
    start) start ;;
    stop) stop ;;
    *) usage ;;
esac
