#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="04-agent"
SENTINEL="$BUILD_DIR/.done/$STAGE_NAME"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# --- Idempotency check ---
if [ -f "$SENTINEL" ]; then
    log "Already complete (sentinel: $SENTINEL) — skipping."
    exit 0
fi

# --- Input validation ---
: "${AGENT_VERSION:?AGENT_VERSION must be set}"
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED:?EMBEDDED must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${NPROC:?NPROC must be set}"
: "${CC:?CC must be set}"
: "${GOPATH:?GOPATH must be set}"
: "${GOROOT:?GOROOT must be set}"
: "${CGO_ENABLED:?CGO_ENABLED must be set}"

# --- Pre-flight: confirm Stage 03 completed ---
if [ ! -f "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-rtloader.so" ]; then
    log "ERROR: libdatadog-agent-rtloader.so not found at $STAGING/opt/datadog-agent/rtloader — did Stage 03 (03-rtloader) complete successfully?"
    exit 1
fi

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -f "$STAGING/opt/datadog-agent/bin/agent"
        rm -f "$STAGING/opt/datadog-agent/bin/trace-agent"
    fi
}
trap cleanup EXIT

# ─── Step 1: Create output directory ──────────────────────────────────────────

log "Creating staging bin directory"
mkdir -p "$STAGING/opt/datadog-agent/bin"

# ─── Step 2: Set rtloader CGO flags ───────────────────────────────────────────
#
# The global CGO_CFLAGS/CGO_LDFLAGS from env.sh point to /opt/freeware headers
# and libs.  We extend them here with rtloader-specific paths so that the Go
# packages that import rtloader (pkg/collector/python/) can find its C headers
# and link against the .so files we built in Stage 03.
#
# Note: we point -L at the BUILD paths (rtloader/build/rtloader and
# rtloader/build/three), not the staging paths.  The .so files are in the build
# tree; the staging copies are for the final package only.

log "Setting rtloader CGO flags"
export CGO_CFLAGS="$CGO_CFLAGS -I/opt/datadog-agent/rtloader/include"
export CGO_LDFLAGS="$CGO_LDFLAGS \
  -L/opt/datadog-agent/rtloader/build/rtloader \
  -L/opt/datadog-agent/rtloader/build/three"

# ─── Step 3: Get commit hash ──────────────────────────────────────────────────
#
# AGENT_COMMIT may be pre-set by the caller (e.g. when the source was transferred
# without the .git directory).  If unset, resolve it from the local git repo.

if [ -n "${AGENT_COMMIT:-}" ]; then
    COMMIT=$AGENT_COMMIT
    log "Using pre-set commit hash: $COMMIT"
elif [ -d /opt/datadog-agent/.git ]; then
    COMMIT=$(git -C /opt/datadog-agent rev-parse --short HEAD)
    log "Resolved commit hash from .git: $COMMIT"
else
    log "ERROR: AGENT_COMMIT env var not set and /opt/datadog-agent/.git not found."
    log "       Set AGENT_COMMIT to the short SHA of the source tree being built,"
    log "       e.g.: AGENT_COMMIT=\$(git -C /path/to/repo rev-parse --short HEAD)"
    exit 1
fi
log "Building agent version $AGENT_VERSION at commit $COMMIT"

# ─── Step 4: Build the agent binary ───────────────────────────────────────────
#
# Build tags used:
#   python            : enable CPython embedding via rtloader
#   otlp              : OpenTelemetry trace/metrics ingestion
#   grpcnotrace       : disable grpc tracing (avoids unnecessary dependency)
#   retrynotrace      : disable retry tracing
#   no_dynamic_plugins: disable dynamic plugin loading (not supported on AIX)
#   trivy_no_javadb   : disable Trivy Java DB (container scanning, not needed)
#   osusergo          : use pure-Go user/group lookups (avoids CGO for getpwuid)
#   datadog.no_waf    : disable the WAF module (Linux-only eBPF dependency)
#   zstd              : enable zstd compression (requires CGO)

log "Building agent binary"
cd /opt/datadog-agent

go build \
    -tags "python otlp grpcnotrace retrynotrace no_dynamic_plugins trivy_no_javadb osusergo datadog.no_waf zstd" \
    -ldflags="-s -w \
      -X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=${AGENT_VERSION} \
      -X github.com/DataDog/datadog-agent/pkg/version.Commit=${COMMIT}" \
    -o "$STAGING/opt/datadog-agent/bin/agent" \
    ./cmd/agent/

log "agent binary build complete: $STAGING/opt/datadog-agent/bin/agent"

# ─── Step 5: Build the trace-agent binary ─────────────────────────────────────
#
# Build tags used:
#   datadog.no_waf : disable the WAF module (Linux-only eBPF dependency)
#   otlp           : OpenTelemetry trace ingestion

log "Building trace-agent binary"
cd /opt/datadog-agent

go build \
    -tags "datadog.no_waf otlp" \
    -ldflags="-s -w \
      -X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=${AGENT_VERSION} \
      -X github.com/DataDog/datadog-agent/pkg/version.Commit=${COMMIT}" \
    -o "$STAGING/opt/datadog-agent/bin/trace-agent" \
    ./cmd/trace-agent/

log "trace-agent binary build complete: $STAGING/opt/datadog-agent/bin/trace-agent"

# ─── Step 6: Verify XCOFF64 magic bytes ───────────────────────────────────────
#
# Both binaries must be XCOFF64 (magic bytes 01 f7).  A non-XCOFF64 result
# would indicate a cross-compile or wrong-format build.

log "Verifying agent binary is XCOFF64"
MAGIC=$(od -A x -t x1 "$STAGING/opt/datadog-agent/bin/agent" | head -1 | awk '{print $2 $3}')
if [ "$MAGIC" != "01f7" ]; then
    log "ERROR: agent binary is not XCOFF64 (got: $MAGIC)"
    log "       Expected magic bytes: 01 f7"
    log "       Ensure CGO_ENABLED=1 and that GOROOT points to the AIX Go toolchain."
    exit 1
fi
log "XCOFF64 magic verified for agent binary (magic: $MAGIC)"

log "Verifying trace-agent binary is XCOFF64"
MAGIC=$(od -A x -t x1 "$STAGING/opt/datadog-agent/bin/trace-agent" | head -1 | awk '{print $2 $3}')
if [ "$MAGIC" != "01f7" ]; then
    log "ERROR: trace-agent binary is not XCOFF64 (got: $MAGIC)"
    log "       Expected magic bytes: 01 f7"
    log "       Ensure CGO_ENABLED=1 and that GOROOT points to the AIX Go toolchain."
    exit 1
fi
log "XCOFF64 magic verified for trace-agent binary (magic: $MAGIC)"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="
