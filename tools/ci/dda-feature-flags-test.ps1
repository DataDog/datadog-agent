# Smoke test for the dda feature flags system (`dda self feature`) on Windows.
# Used by the dda_feature_flags_test_windows dummy CI job, run inside the winbuild
# docker image. It validates that feature flags are actually evaluated against the
# remote feature flag service, and not silently falling back to their default value.

$ErrorActionPreference = "Stop"

$platform = "windows"
$flag = if ($env:DDA_FEATURE_FLAGS_TEST_FLAG) { $env:DDA_FEATURE_FLAGS_TEST_FLAG } else { "dd-agent-test" }
$dummyFlag = "dda-feature-flags-smoke-test-flag-$platform-do-not-create"

Write-Host "=== dda feature flags smoke test ($platform) ==="

# --- Test 1: evaluate a real flag, it must NOT fall back to the default value ---
Write-Host "--- [1/3] Evaluating real flag '$flag' (must not fall back to default) ---"
$res = (dda self feature $flag --json | Out-String).Trim()
Write-Host "dda self feature $flag --json -> $res"
$res = $res -replace '\s', '' # strip spaces to make the checks below robust to pretty-printed JSON
if ($res -notmatch '"defaulted":false') {
    Write-Host "FAIL: the evaluation of '$flag' fell back to the default value; the feature flag service could not be reached or the flag does not exist."
    exit 1
}
if ($res -notmatch '"error":null') {
    Write-Host "FAIL: the evaluation of '$flag' returned an error."
    exit 1
}
Write-Host "PASS: '$flag' was evaluated against the feature flag service."

# --- Test 2: evaluate a flag that does not exist, it must fall back to the given default ---
Write-Host "--- [2/3] Evaluating unknown flag '$dummyFlag' (must fall back to given default) ---"
$res = (dda self feature $dummyFlag --default true --json | Out-String).Trim()
Write-Host "dda self feature $dummyFlag --default true --json -> $res"
$res = $res -replace '\s', ''
if (($res -notmatch '"value":true') -or ($res -notmatch '"defaulted":true')) {
    Write-Host "FAIL: the evaluation of '$dummyFlag' did not fall back to the requested default value."
    exit 1
}
Write-Host "PASS: the default fallback works as expected."

# --- Test 3: check the plain output contract used by tasks/libs/common/feature_flags.py ---
Write-Host "--- [3/3] Checking plain output contract ('True' or 'False') ---"
$plain = (dda self feature $flag | Out-String).Trim()
Write-Host "dda self feature $flag -> $plain"
if (($plain -ne "True") -and ($plain -ne "False")) {
    Write-Host "FAIL: plain output must be 'True' or 'False', got '$plain'."
    exit 1
}
Write-Host "PASS: plain output matches the contract expected by tasks/libs/common/feature_flags.py."

Write-Host "=== dda feature flags smoke test ($platform): all tests passed ==="
