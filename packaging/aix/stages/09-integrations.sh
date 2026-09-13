#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="09-integrations"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# --- Input validation ---
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"
: "${WHEEL_CACHE:?WHEEL_CACHE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip${PYTHON_MAJ_MIN}

# --- Pre-flight checks ---
if [ ! -x "$PIP" ]; then
    log "ERROR: $PIP not found — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

if [ ! -f "$STAGING/constraints.txt" ]; then
    log "ERROR: $STAGING/constraints.txt not found — Stage 08 (08-checks-base) must complete first"
    exit 1
fi

# --- Cleanup on failure ---
# pip installs are not easy to roll back; the sentinel not being written is
# sufficient to trigger a re-run.
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        log "       Common causes:"
        log "         - Stage 08 constraints.txt missing: ensure Stage 08 completed"
        log "         - integrations-core check not found: verify INTEGRATIONS_CORE=$INTEGRATIONS_CORE"
        log "         - Network access required for any dep not yet in site-packages"
    fi
}
trap cleanup EXIT

# ─── Step 1: Supplement Go check configs from integrations-core ──────────────
#
# For every Go check whose conf.d directory was populated by inv agent.build,
# also copy conf.yaml.default and conf.yaml.example from integrations-core if
# they exist there. This provides the full documented configuration (e.g. snmp
# has a rich conf.yaml.example in integrations-core but only an auto_conf.yaml
# in the agent repo). Stage 11 copies agent-repo configs afterward, so
# agent-repo files take precedence when both repos provide the same filename.

AGENT_DIST_CONFD="$AGENT_SRC/bin/agent/dist/conf.d"
if [ -d "$AGENT_DIST_CONFD" ]; then
    for check_dir in "$AGENT_DIST_CONFD"/*.d; do
        [ -d "$check_dir" ] || continue
        check=$(basename "$check_dir" .d)
        CHECK_DATA="$INTEGRATIONS_CORE/$check/datadog_checks/$check/data"
        if [ -d "$CHECK_DATA" ]; then
            mkdir -p "$STAGING/etc/datadog-agent/conf.d/${check}.d"
            for conf_file in conf.yaml.example conf.yaml.default; do
                if [ -f "$CHECK_DATA/$conf_file" ]; then
                    cp "$CHECK_DATA/$conf_file" "$STAGING/etc/datadog-agent/conf.d/${check}.d/"
                    log "Copied integrations-core $conf_file for Go check: $check"
                fi
            done
        fi
    done
fi

# ─── Step 2: Install Python checks from integrations-core ─────────────────────
#
# Discover every check in the pinned integrations-core checkout whose
# manifest.json declares "Supported OS::AIX" in tile.classifier_tags, then
# install each one. This mirrors how deps/agent_integrations/source_packages.bzl
# selects checks for the other platforms (Supported OS::Linux/macOS/Windows),
# so integrations-core is the single source of truth for AIX support instead
# of a hardcoded list here.
#
# Dependency versions come from integrations-core's agent_requirements.in — the
# same pinned input file the Linux/macOS/Windows lockfile flow (resolve-build-deps)
# compiles into the per-platform .deps/resolved/*.txt wheel sets. AIX has no
# such lockfile (no prebuilt AIX wheels exist), so instead of compiling one we
# install the AIX-relevant subset of agent_requirements.in directly: the union
# of every AIX-tagged check's [deps] extra, looked up in agent_requirements.in
# for the canonical pin. Native deps Stage 06 already built and installed
# (pymqi, lxml, psutil, cryptography) are seen as satisfied and not rebuilt;
# only missing pure-Python deps (e.g. http_check's pysocks/requests-ntlm) are
# fetched from PyPI.
#
# Native C-extension deps that Stage 06 did NOT build (e.g. pyodbc when
# unixODBC headers are absent) are filtered out so the dep install does not
# fail on a source build that cannot succeed. The check still installs; it
# surfaces a clear ImportError at runtime if the missing extension is needed,
# matching the graceful-degradation behavior for the IBM checks.
#
# --constraint pins all transitive deps to the exact versions frozen by Stage 08.
# --find-links allows pip to locate native AIX wheels (pydantic-core, cryptography)
# from the local cache if needed rather than hitting PyPI.

PYTHON_CHECKS=$(python3.12 -c "
import json
import os

checks = []
for name in sorted(os.listdir('$INTEGRATIONS_CORE')):
    check_dir = os.path.join('$INTEGRATIONS_CORE', name)
    if not os.path.isfile(os.path.join(check_dir, 'pyproject.toml')):
        continue
    manifest_path = os.path.join(check_dir, 'manifest.json')
    if not os.path.isfile(manifest_path):
        continue
    with open(manifest_path) as f:
        manifest = json.load(f)
    tags = manifest.get('tile', {}).get('classifier_tags', [])
    if 'Supported OS::AIX' in tags:
        checks.append(name)
print(' '.join(checks))
")

if [ -z "$PYTHON_CHECKS" ]; then
    log "ERROR: no integrations-core checks found with the Supported OS::AIX manifest tag"
    exit 1
fi

log "Discovered Python checks tagged Supported OS::AIX: $PYTHON_CHECKS"

# Build the AIX dependency subset from agent_requirements.in: the union of the
# [deps] extras of every AIX-tagged check, pinned to agent_requirements.in's
# versions. This is the same set the other platforms resolve into their lockfiles;
# we install it directly because AIX has no prebuilt wheel set.
AIX_DEPS="$BUILD_DIR/.09-aix-deps.tmp"
AGENT_REQ="$INTEGRATIONS_CORE/agent_requirements.in"
if [ ! -f "$AGENT_REQ" ]; then
    log "ERROR: $AGENT_REQ not found — is the integrations-core checkout complete?"
    exit 1
fi
python3.12 - "$INTEGRATIONS_CORE" "$AGENT_REQ" "$STAGING/constraints.txt" "$AIX_DEPS" <<'PYEOF'
import json, os, re, sys, tomllib
ic, req_in, constraints, out = sys.argv[1:5]
# Native C extensions Stage 06 builds conditionally on host prerequisites.
# If absent from the frozen constraints (i.e. not built), skip them rather
# than fail the install with a source build that cannot succeed.
NATIVE_OPTIONAL = {"pyodbc"}

def pkg_name(spec):
    return re.split(r"[<>=!;\[]", spec.strip(), 1)[0].strip().lower()

# 1. Collect the dep package names declared by every AIX-tagged check's
#    [deps] extra. These are the only deps the AIX package needs.
needed = set()
for name in sorted(os.listdir(ic)):
    pp = os.path.join(ic, name, "pyproject.toml")
    mp = os.path.join(ic, name, "manifest.json")
    if not (os.path.isfile(pp) and os.path.isfile(mp)):
        continue
    with open(mp) as f:
        if "Supported OS::AIX" not in json.load(f).get("tile", {}).get("classifier_tags", []):
            continue
    with open(pp, "rb") as f:
        t = tomllib.load(f)
    for d in t.get("project", {}).get("optional-dependencies", {}).get("deps", []):
        needed.add(pkg_name(d))

# 2. Drop native deps Stage 06 did not build (absent from the freeze).
installed = set()
with open(constraints) as f:
    for line in f:
        n = pkg_name(line)
        if n:
            installed.add(n)
for n in sorted(needed):
    if n in NATIVE_OPTIONAL and n not in installed:
        print(f"# skipped (native, not built): {n}", file=sys.stderr)
        needed.discard(n)

# 3. Emit the matching lines from agent_requirements.in (canonical pins).
#    pip evaluates each line's environment markers, so win32/darwin-marked
#    lines that slipped into `needed` are skipped on AIX automatically.
missing = set(needed)
with open(req_in) as f, open(out, "w") as w:
    for line in f:
        n = pkg_name(line)
        if n in needed:
            w.write(line)
            missing.discard(n)
for n in sorted(missing):
    print(f"# WARNING: {n} needed by an AIX check but not in agent_requirements.in", file=sys.stderr)
PYEOF
log "AIX dependency subset written to $AIX_DEPS ($(wc -l < "$AIX_DEPS" | tr -d ' ') entries):"
sed 's/^/  /' "$AIX_DEPS" >&2

log "Installing AIX dependency subset from agent_requirements.in"
$PIP install \
    --constraint "$STAGING/constraints.txt" \
    --find-links "$WHEEL_CACHE" \
    -r "$AIX_DEPS"
rm -f "$AIX_DEPS"
log "AIX dependency subset installed"

# Install each check's own code. Its [deps] extra deps are already installed
# above, so a plain 'pip install <check_dir>' suffices and will not rebuild
# native extensions.
for check in $PYTHON_CHECKS; do
    CHECK_DIR="$INTEGRATIONS_CORE/$check"
    log "Installing check: $check"
    $PIP install \
        --constraint "$STAGING/constraints.txt" \
        --find-links "$WHEEL_CACHE" \
        "$CHECK_DIR"
    mkdir -p "$STAGING/etc/datadog-agent/conf.d/${check}.d"
    EXAMPLE="$CHECK_DIR/datadog_checks/$check/data/conf.yaml.example"
    if [ -f "$EXAMPLE" ]; then
        cp "$EXAMPLE" "$STAGING/etc/datadog-agent/conf.d/${check}.d/"
    else
        log "WARNING: no conf.yaml.example found for $check at $EXAMPLE"
    fi
    log "Check $check installed successfully"
done

log "All Python checks processed"

log "=== $STAGE_NAME complete ==="
