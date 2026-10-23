#!/bin/bash
# Smoke test for the dda feature flags system (`dda self feature`).
# Used by the dda_feature_flags_test_linux and dda_feature_flags_test_macos dummy CI jobs.
# It validates that feature flags are actually evaluated against the remote feature
# flag service, and not silently falling back to their default value.

set -eu

platform="$(uname -s)"
flag="${DDA_FEATURE_FLAGS_TEST_FLAG:-dd-agent-test}"
dummy_flag="dda-feature-flags-smoke-test-flag-${platform}-do-not-create"

echo "=== dda feature flags smoke test (${platform}) ==="

# --- Test 1: evaluate a real flag, it must NOT fall back to the default value ---
echo "--- [1/3] Evaluating real flag '${flag}' (must not fall back to default) ---"
res="$(dda self feature "${flag}" --json)"
echo "dda self feature ${flag} --json -> ${res}"
res="${res// /}" # strip spaces to make the checks below robust to pretty-printed JSON
if [[ "${res}" != *'"defaulted":false'* ]]; then
    echo "❌ FAIL: the evaluation of '${flag}' fell back to the default value; the feature flag service could not be reached or the flag does not exist."
    exit 1
fi
if [[ "${res}" != *'"error":null'* ]]; then
    echo "❌ FAIL: the evaluation of '${flag}' returned an error."
    exit 1
fi
echo "✅ PASS: '${flag}' was evaluated against the feature flag service."

# --- Test 2: evaluate a flag that does not exist, it must fall back to the given default ---
echo "--- [2/3] Evaluating unknown flag '${dummy_flag}' (must fall back to given default) ---"
res="$(dda self feature "${dummy_flag}" --default true --json)"
echo "dda self feature ${dummy_flag} --default true --json -> ${res}"
res="${res// /}"
if [[ "${res}" != *'"value":true'* ]] || [[ "${res}" != *'"defaulted":true'* ]]; then
    echo "❌ FAIL: the evaluation of '${dummy_flag}' did not fall back to the requested default value."
    exit 1
fi
echo "✅ PASS: the default fallback works as expected."

# --- Test 3: check the plain output contract used by tasks/libs/common/feature_flags.py ---
echo "--- [3/3] Checking plain output contract ('True' or 'False') ---"
plain="$(dda self feature "${flag}")"
echo "dda self feature ${flag} -> ${plain}"
if [[ "${plain}" != "True" && "${plain}" != "False" ]]; then
    echo "❌ FAIL: plain output must be 'True' or 'False', got '${plain}'."
    exit 1
fi
echo "✅ PASS: plain output matches the contract expected by tasks/libs/common/feature_flags.py."

echo "=== dda feature flags smoke test (${platform}): all tests passed ==="
