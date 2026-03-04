#!/bin/sh
set -eu

# Source shared environment (defines STAGING, BUILD_DIR, AGENT_VRMF, log(), etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/lib/env.sh"

STAGE_NAME="package"
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
: "${AGENT_BUILD:?AGENT_BUILD must be set}"
: "${AGENT_VRMF:?AGENT_VRMF must be set}"
: "${STAGING:?STAGING must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"

# --- Output artifact path ---
BFF_OUT="$BUILD_DIR/datadog-agent-${AGENT_VERSION}-${AGENT_BUILD}.aix.ppc64.bff"

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -f "$BFF_OUT"
    fi
}
trap cleanup EXIT

# ─── Step 1: Pre-flight — verify staging tree is assembled ────────────────────
#
# The agent binary and the postinst lifecycle script are the two most critical
# components.  If either is absent the staging tree is incomplete and mkinstallp
# will produce a broken or empty BFF.

AGENT_BIN="$STAGING/opt/datadog-agent/bin/agent"
if [ ! -f "$AGENT_BIN" ]; then
    log "ERROR: agent binary not found at $AGENT_BIN"
    log "       Did Stage 04 (04-agent) complete successfully?"
    exit 1
fi
log "Pre-flight: agent binary found at $AGENT_BIN"

POSTINST="$STAGING/opt/datadog-agent/embedded/share/installp/postinst"
if [ ! -f "$POSTINST" ]; then
    log "ERROR: postinst script not found at $POSTINST"
    log "       Did Stage 10 (10-assemble) complete successfully?"
    exit 1
fi
log "Pre-flight: postinst script found at $POSTINST"

# ─── Step 2: Generate the mkinstallp template with VRMF and full file list ────
#
# gen.template.in contains two placeholder tokens:
#   AGENT_VRMF_PLACEHOLDER — replaced with the four-component VRMF (e.g. 7.78.0.1)
#   __FILE_LIST__          — replaced with a complete list of all files/dirs to package
#
# AIX mkinstallp USRFiles requires individual file/directory paths; listing a
# directory path alone only packages the directory entry, not its contents.
# We use find to enumerate every path under each package directory and strip the
# staging prefix so each line is the absolute installed path (e.g. /opt/datadog-agent/bin/agent).

log "Generating gen.template with VRMF=${AGENT_VRMF}"

FILELIST="$BUILD_DIR/.pkg_filelist.tmp"
: > "$FILELIST"
for pkg_dir in opt/datadog-agent etc/datadog-agent var/log/datadog var/run/datadog; do
    if [ -d "$STAGING/$pkg_dir" ]; then
        # Filter out paths with spaces or parentheses: AIX mkinstallp splits USRFiles lines
        # on whitespace, so filenames containing spaces cause checksum errors and are skipped.
        # Files like "Lorem ipsum.txt" and "launcher manifest.xml" (setuptools vendor docs)
        # are documentation only and not needed at runtime.
        find "$STAGING/$pkg_dir" -print | sed "s|^$STAGING||" | grep -v '[ ()]' >> "$FILELIST"
    fi
done
FILE_COUNT=$(wc -l < "$FILELIST" | tr -d ' ')
log "File list generated: $FILE_COUNT paths"

# Build gen.template: substitute VRMF and expand __FILE_LIST__ with the file list
{
    while IFS= read -r tmpl_line; do
        case "$tmpl_line" in
            '__FILE_LIST__')
                cat "$FILELIST"
                ;;
            *'AGENT_VRMF_PLACEHOLDER'*)
                printf '%s\n' "$tmpl_line" | sed "s/AGENT_VRMF_PLACEHOLDER/${AGENT_VRMF}/g"
                ;;
            *)
                printf '%s\n' "$tmpl_line"
                ;;
        esac
    done < "$SCRIPT_DIR/gen.template.in"
} > "$STAGING/gen.template"
log "Template written to $STAGING/gen.template ($FILE_COUNT file entries)"

# ─── Step 3: Run mkinstallp to generate the BFF ───────────────────────────────
#
# mkinstallp writes its output to $STAGING/tmp/datadog-agent.<VRMF>.bff.
# The -d flag specifies the staging root (all paths in USRFiles are relative to
# this directory).  The -T flag specifies the template file.
#
# mkinstallp requires the tmp/ subdirectory to exist; create it explicitly so
# the error message is clear if it fails rather than getting a cryptic mkdir
# failure from inside mkinstallp.
#
# Remove any .info directory left by a previous failed mkinstallp run; mkinstallp
# refuses to overwrite its own control files and exits with error 0503-844.

log "Running mkinstallp (this may take a few minutes for large packages)"
rm -rf "$STAGING/.info"
mkdir -p "$STAGING/tmp"
/usr/sbin/mkinstallp -d "$STAGING" -T "$STAGING/gen.template"
log "mkinstallp completed"

# ─── Step 4: Copy BFF to final artifact name ──────────────────────────────────
#
# mkinstallp names the output file after the package name and VRMF.
# Rename to the canonical artifact name used by CI and release tooling:
#   datadog-agent-<AGENT_VERSION>-<AGENT_BUILD>.aix.ppc64.bff
# e.g. datadog-agent-7.78.0-1.aix.ppc64.bff

BFF_SRC="$STAGING/tmp/datadog-agent.${AGENT_VRMF}.bff"
if [ ! -f "$BFF_SRC" ]; then
    log "ERROR: expected BFF not found at $BFF_SRC"
    log "       Check mkinstallp output above for errors."
    exit 1
fi

cp "$BFF_SRC" "$BFF_OUT"
ls -l "$BFF_OUT"
log "Package ready: $BFF_OUT"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="
