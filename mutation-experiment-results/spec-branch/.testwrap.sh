#!/usr/bin/env bash
unset GOTMPDIR
export GOWORK=off
cd "$MUT_REPO_ROOT" || exit 2
rapid_flags=""
if grep -rq "pgregory.net/rapid" "./$MUT_TARGET"/*_test.go 2>/dev/null; then
  rapid_flags="-rapid.seed=${MUT_RAPID_SEED:-1} -rapid.checks=${MUT_RAPID_CHECKS:-1000}"
fi
out=$(go test -tags=test -count=1 \
  -timeout "${MUT_TIMEOUT:-120}s" \
  "./$MUT_TARGET" $rapid_flags 2>&1)
rc=$?
if printf '%s' "$out" | grep -q "test timed out"; then t=1; else t=0; fi
printf 'rc=%s timeout=%s\n' "$rc" "$t" >> "$MUT_LOG"
exit $rc
