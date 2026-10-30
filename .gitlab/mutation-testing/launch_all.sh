#!/usr/bin/env bash
# ABOUTME: Launches the full Logs Agent gremlins mutation sweep in a detached tmux session.
# ABOUTME: Safe to disconnect after; re-attach with `tmux attach -t logs-muttest`. Resumable.
set -euo pipefail

SESSION="${SESSION:-logs-muttest}"
RESULTS_DIR="${RESULTS_DIR:-$HOME/research/logs-agent-mutation-results}"
REPO="${REPO:-$HOME/repos/datadog-agent}"
SCRIPT="${SCRIPT:-$(dirname "$0")/run_mutation.py}"
SCOPE="${SCOPE:-all}"
PER_PKG_TIMEOUT="${PER_PKG_TIMEOUT:-7200}"
TEST_TIMEOUT="${TEST_TIMEOUT:-60}"

# dda must be on PATH inside tmux. A detached tmux shell may not be a login
# shell, so inject the launching shell's PATH plus the usual dda/Go bin dirs.
# Override with DDA_BIN_DIR if dda lives elsewhere on this host.
EXTRA_PATH="${DDA_BIN_DIR:-}:$HOME/.local/bin:/usr/local/bin:/opt/homebrew/bin:$HOME/go/bin"
EXPORT_PATH="${EXTRA_PATH}:$PATH"

mkdir -p "$RESULTS_DIR"
LOG="$RESULTS_DIR/run.log"

if tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "tmux session '$SESSION' already exists. Attach with: tmux attach -t $SESSION"
    exit 1
fi

CMD="export PATH=$EXPORT_PATH; cd $REPO && python3 $SCRIPT --repo $REPO --results-dir $RESULTS_DIR --scope $SCOPE --per-package-timeout $PER_PKG_TIMEOUT --test-timeout $TEST_TIMEOUT 2>&1 | tee -a $LOG; echo; echo DONE; read -p 'Press enter to close...' _"

tmux new-session -d -s "$SESSION" "$CMD"

echo "Launched tmux session '$SESSION'."
echo "  Log:    $LOG"
echo "  Attach: tmux attach -t $SESSION"
echo "  Status: tail -f $LOG   OR   wc -l $RESULTS_DIR/status.jsonl"
