#!/usr/bin/env bash
# Download, start, and stop bazel-conduit for a CI Bazel job.
# start: fetch the pinned Linux x86_64 release, serve BES on 127.0.0.1:8080,
#        export OTLP to the node Agent at http://$BUILDENV_HOST_IP:4317.
# stop:  SIGTERM the server and wait for the exporter flush.
# metadata: print build --build_metadata lines for user.bazelrc.
set -euo pipefail

CONDUIT_VERSION="${CONDUIT_VERSION:-0.0.1-RC2}"
# sha256 of conduit-linux-x86_64 from the v0.0.1-RC2 GitHub release.
CONDUIT_SHA256="${CONDUIT_SHA256:-d657f43c95c7b1e419b5a1e315f1b65f8b6ef5be2c2c4fc554ac4bbfc240ae00}"
CONDUIT_ASSET="conduit-linux-x86_64"
# Depot read API caches GitHub release assets in ADMS; curl does not use the Bazel mirror rewrite.
CONDUIT_URL="${CONDUIT_URL:-https://depot-read-api-bzl.us1.ddbuild.io/github.com/JSGette/bazel_conduit/releases/download/v${CONDUIT_VERSION}/${CONDUIT_ASSET}}"
# Must match the bes_backend port written to user.bazelrc by the coverage job.
CONDUIT_PORT=8080
CONDUIT_LISTEN_TIMEOUT_S="${CONDUIT_LISTEN_TIMEOUT_S:-5}"
CONDUIT_STOP_TIMEOUT_S="${CONDUIT_STOP_TIMEOUT_S:-15}"

# Paths need CI_PROJECT_DIR. metadata does not, so this stays off the top level.
prepare_paths() {
    CONDUIT_DIR="${CONDUIT_DIR:-${CI_PROJECT_DIR:?CI_PROJECT_DIR is required}/.conduit}"
    CONDUIT_BIN="${CONDUIT_DIR}/${CONDUIT_ASSET}"
    CONDUIT_PID_FILE="${CONDUIT_DIR}/conduit.pid"
    CONDUIT_LOG="${CONDUIT_DIR}/conduit.log"
}

usage() {
    echo "usage: $0 start|stop|metadata" >&2
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

# Bazelrc treats '#' as a comment and splits on whitespace, and conduit does not
# redact --build_metadata. Skip a value instead of writing a broken or leaky line.
emit_metadata() {
    local key="$1" value="$2"
    if [[ -z "${value}" ]]; then
        return 0
    fi
    if [[ "${value}" == *[!A-Za-z0-9._:/-]* ]]; then
        echo "skip metadata ${key}: unsupported characters" >&2
        return 0
    fi
    echo "build --build_metadata=${key}=${value}"
}

metadata() {
    # Detached CI checkouts have a commit and no branch. Prefer the PR branch.
    local branch pr_id
    branch="${CI_EXTERNAL_PULL_REQUEST_SOURCE_BRANCH_NAME:-${CI_MERGE_REQUEST_SOURCE_BRANCH_NAME:-${CI_COMMIT_BRANCH:-${CI_COMMIT_REF_NAME:-}}}}"
    pr_id="${CI_EXTERNAL_PULL_REQUEST_IID:-${CI_MERGE_REQUEST_IID:-}}"

    emit_metadata REPO_URL "https://github.com/DataDog/datadog-agent"
    emit_metadata COMMIT_SHA "${CI_COMMIT_SHA:-}"
    emit_metadata BRANCH "${branch}"
    emit_metadata CI_PIPELINE_ID "${CI_PIPELINE_ID:-}"
    emit_metadata CI_JOB_ID "${CI_JOB_ID:-}"
    emit_metadata CI_JOB_NAME "${CI_JOB_NAME:-}"
    emit_metadata ARCH "$(uname -m)"
    emit_metadata PR_ID "${pr_id}"
}

start() {
    if [[ -z "${BUILDENV_HOST_IP:-}" ]]; then
        echo "BUILDENV_HOST_IP is unset; cannot export OTLP to the node Agent" >&2
        exit 1
    fi

    prepare_paths
    mkdir -p "${CONDUIT_DIR}"
    # GitHub release fetches flake on transient network/5xx; retry before aborting coverage.
    "${CI_PROJECT_DIR}/tools/ci/retry.sh" -n 5 curl -fsSL -o "${CONDUIT_BIN}" "${CONDUIT_URL}"
    echo "${CONDUIT_SHA256}  ${CONDUIT_BIN}" | sha256sum -c -
    chmod +x "${CONDUIT_BIN}"

    "${CONDUIT_BIN}" --version

    local otlp_endpoint="http://${BUILDENV_HOST_IP}:4317"
    : >"${CONDUIT_LOG}"
    # info logs one line per target and test; stop cats this file into the job log.
    nohup "${CONDUIT_BIN}" serve \
        --export otlp \
        --otlp-endpoint "${otlp_endpoint}" \
        --port "${CONDUIT_PORT}" \
        --log-level warn \
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
    prepare_paths
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
    metadata) metadata ;;
    *) usage ;;
esac
