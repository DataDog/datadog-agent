#!/usr/bin/env bash
# Workspace status script for Bazel stamping.
# Outputs key-value pairs (space-separated, one per line) used by --stamp builds.
# Keys prefixed with STABLE_ trigger a re-link when their value changes.
# Non-stable keys only invalidate volatile status (not the analysis cache).
#
# Mirrors the version-string format produced by tasks/libs/releasing/version.py
# get_version(include_git=True): "<major>.<minor>.<patch>[-<pre>]+git.<N>.<sha>"

set -euo pipefail

# Short commit SHA, matching omnibus get_commit_sha(short=True)
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Version string formatted to match omnibus get_version(include_git=True):
#   git describe raw:  7.81.0-devel-561-gedd39ec
#   omnibus output:    7.81.0-devel+git.561.edd39ec
# If we are exactly on a tag, no +git suffix is appended.
RAW=$(git describe --tags --candidates=50 --match "[0-9]*.*" --abbrev=7 2>/dev/null || echo "0.0.0-dev")
VERSION=$(echo "${RAW}" | python3 -c "
import re, sys
described = sys.stdin.read().strip()
# Extract N and sha from trailing -N-g<sha>
m = re.match(r'^(.*?)-(\d+)-g([0-9a-f]+)$', described)
if m:
    base, n, sha = m.group(1), m.group(2), m.group(3)
    print(f'{base}+git.{n}.{sha}')
else:
    print(described)
" 2>/dev/null || echo "${RAW}")

# URL-safe variant (+ replaced by .): matches omnibus get_version(url_safe=True)
VERSION_URL_SAFE="${VERSION//+/.}"

# Agent payload version from go.mod (matches get_payload_version() in tasks/libs/common/utils.py)
PAYLOAD_VERSION=$(grep 'github.com/DataDog/agent-payload/v5' go.mod | head -1 | awk '{print $2}' | cut -d- -f1)

echo "STABLE_GIT_COMMIT ${COMMIT}"
echo "STABLE_AGENT_VERSION ${VERSION}"
echo "STABLE_AGENT_VERSION_URL_SAFE ${VERSION_URL_SAFE}"
echo "STABLE_AGENT_PAYLOAD_VERSION ${PAYLOAD_VERSION}"
