// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// maxAttackAllocBytes bounds the total allocation of the memory-exhaustion
// scripts below. Without the fixes each one allocates hundreds of megabytes
// or more; bounded, each stays within a few megabytes of real work.
const maxAttackAllocBytes = 64 << 20

// runMeasured runs script with exactly opts (no commands are allowed unless
// opts allow them) and returns its output and the bytes allocated while it
// ran. The tests using it are not parallel, so no other test allocates
// concurrently.
func runMeasured(t *testing.T, script string, stdin io.Reader, stderr io.Writer, opts ...interp.RunnerOption) (stdout string, totalAlloc uint64) {
	t.Helper()
	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var outBuf bytes.Buffer
	runner, err := interp.New(append([]interp.RunnerOption{
		interp.StdIO(stdin, &outBuf, stderr),
		interp.MaxExecutionTime(30 * time.Second),
	}, opts...)...)
	require.NoError(t, err)
	defer runner.Close()

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_ = runner.Run(context.Background(), prog)
	runtime.ReadMemStats(&after)
	return outBuf.String(), after.TotalAlloc - before.TotalAlloc
}

// spacedValueScript assigns x a 512 KiB value made of single-character words
// separated by spaces: 262,144 fields when split.
const spacedValueScript = "x='a '\n" +
	"for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18; do x=\"$x$x\"; done\n"

// TestVULN4760BraceExpansionBoundedBeforeCommandGate pins that a compact brace
// range neither materializes millions of fields before the command gate nor
// overruns the brace limit for an allowed command.
func TestVULN4760BraceExpansionBoundedBeforeCommandGate(t *testing.T) {
	for name, tc := range map[string]struct {
		script string
		stderr string
	}{
		"command not allowed": {"not_allowed {1..3000000}\n", "not_allowed: command not allowed"},
		"command allowed":     {"echo {1..3000000}\n", "brace expansion would exceed"},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			start := time.Now()
			_, alloc := runMeasured(t, tc.script, nil, &stderr,
				interp.AllowedCommands([]string{"rshell:echo"}))

			assert.Contains(t, stderr.String(), tc.stderr)
			assert.Less(t, alloc, uint64(maxAttackAllocBytes))
			assert.Less(t, time.Since(start), 5*time.Second)
		})
	}
}

// TestVULN5664UnquotedSplitBoundedBeforeExpansion pins that the field cap is
// enforced on an upper bound before an unquoted word is split, not after the
// expander has built every field. No command is allowed: the word is the
// command name.
func TestVULN5664UnquotedSplitBoundedBeforeExpansion(t *testing.T) {
	for name, word := range map[string]string{
		"command":  strings.Repeat("$x", 20) + "\n",
		"for-loop": "for v in " + strings.Repeat("$x", 20) + "; do :; done\n",
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			stdout, alloc := runMeasured(t, spacedValueScript+word+"echo AFTER\n", nil, &stderr)

			assert.Contains(t, stderr.String(), "expansion exceeds maximum field count")
			assert.NotContains(t, stdout, "AFTER")
			assert.Less(t, alloc, uint64(maxAttackAllocBytes))
		})
	}
}

// TestVULN5664UnquotedCommandSubstitutionSplitBounded covers the same
// allocate-then-check gap for unquoted command substitutions: their output is
// charged against the field cap before the expander splits it.
func TestVULN5664UnquotedCommandSubstitutionSplitBounded(t *testing.T) {
	var stderr bytes.Buffer
	stdout, alloc := runMeasured(t,
		spacedValueScript+`echo $(echo "$x")$(echo "$x")$(echo "$x")$(echo "$x")$(echo "$x") >/dev/null`+"\necho AFTER\n",
		nil, &stderr, interp.AllowedCommands([]string{"rshell:echo"}))

	assert.Contains(t, stderr.String(), "expansion exceeds maximum field count")
	assert.NotContains(t, stdout, "AFTER")
	assert.Less(t, alloc, uint64(maxAttackAllocBytes))
}

// TestVULN5630EmptyVariablesChargedAgainstStorageCap pins that variable names
// and entries count against MaxTotalVarsBytes, so empty-valued variables
// created by read cannot grow storage without bound.
func TestVULN5630EmptyVariablesChargedAgainstStorageCap(t *testing.T) {
	var stderr bytes.Buffer
	stdout, _ := runMeasured(t,
		"for p in a b c d e f; do read $p{a..y}{a..y}{a..z} <<E\nE\ndone\necho AFTER\n",
		nil, &stderr, interp.AllowedCommands([]string{"rshell:read", "rshell:echo"}))

	assert.Contains(t, stderr.String(), "variable storage limit exceeded")
	assert.NotContains(t, stdout, "AFTER")
}

// TestVULN5630EmptyAssignmentsChargedAgainstStorageCap is the variant that
// needs no allowed command: plain empty assignments.
func TestVULN5630EmptyAssignmentsChargedAgainstStorageCap(t *testing.T) {
	var sb strings.Builder
	for i := range 20000 {
		fmt.Fprintf(&sb, "v%d=\n", i)
	}
	sb.WriteString("echo AFTER\n")
	var stderr bytes.Buffer
	stdout, _ := runMeasured(t, sb.String(), nil, &stderr,
		interp.AllowedCommands([]string{"rshell:echo"}))

	assert.Contains(t, stderr.String(), "variable storage limit exceeded")
	assert.NotContains(t, stdout, "AFTER")
}

// TestVULN5664SubstitutionCountedAfterNormalization pins that unquoted
// command substitution output is charged as the expander splits it: with NUL
// bytes removed. Removing a NUL inside a multi-byte IFS character creates a
// delimiter the raw bytes do not contain, so the field cap must reject the
// word before the following substitution runs.
func TestVULN5664SubstitutionCountedAfterNormalization(t *testing.T) {
	var stderr bytes.Buffer
	stdout, _ := runMeasured(t,
		"IFS=\u00e9; true {1..16383} $(printf 'a\\303\\000\\251a')$(echo REACHED >&2); echo AFTER\n",
		nil, &stderr, interp.AllowedCommands([]string{"rshell:printf", "rshell:echo", "rshell:true"}))
	assert.Contains(t, stderr.String(), "expansion exceeds maximum field count")
	assert.NotContains(t, stdout, "AFTER")
	assert.NotContains(t, stdout+stderr.String(), "REACHED")
}

// signalWriter collects output and closes found once it contains substr.
type signalWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	substr string
	found  chan struct{}
	once   sync.Once
}

func (w *signalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if strings.Contains(w.buf.String(), w.substr) {
		w.once.Do(func() { close(w.found) })
	}
	return len(p), nil
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestVULN1675ConcurrentPipelineStagesShareStorageCap pins that concurrent
// pipeline stages draw on one run-wide variable budget. Each stage builds a
// 512 KiB variable and then blocks in cat, so all of them are live at once;
// three of them exceed MaxTotalVarsBytes together.
func TestVULN1675ConcurrentPipelineStagesShareStorageCap(t *testing.T) {
	stage := `{ X=AAAAAAAAAAAAAAAA; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; ` +
		`X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; cat; }`
	script := strings.Join([]string{stage, stage, stage}, " | ") + "\n"

	// The first stage's cat holds the pipeline open until the cap fires (or,
	// without the fix, until the timeout below), keeping every stage live.
	stdinR, stdinW := io.Pipe()
	stderr := &signalWriter{substr: "variable storage limit exceeded", found: make(chan struct{})}
	go func() {
		select {
		case <-stderr.found:
		case <-time.After(5 * time.Second):
		}
		stdinW.Close()
	}()
	runMeasured(t, script, stdinR, stderr, interp.AllowedCommands([]string{"rshell:cat"}))

	assert.Contains(t, stderr.String(), "variable storage limit exceeded")
}

// TestMemoryAccountingNormalWorkloads checks that ordinary scripts stay within
// the limits: storage released by finished subshells and pipeline stages is
// reusable, and moderate splitting and variable counts are unaffected.
func TestMemoryAccountingNormalWorkloads(t *testing.T) {
	big := "X=AAAAAAAAAAAAAAAA; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; " +
		"X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X; X=$X$X"
	words := strings.TrimSpace(strings.Repeat("w ", 2000))
	tests := map[string]struct {
		script string
		want   string
	}{
		"sequential pipelines release storage": {
			script: "for i in {1..40}; do { " + big + "; echo ok; } | cat >/dev/null; done; echo DONE\n",
			want:   "DONE\n",
		},
		"command substitutions release storage": {
			script: "for i in {1..40}; do y=$(" + big + "; echo ok); done; echo $y DONE\n",
			want:   "ok DONE\n",
		},
		"subshells release storage": {
			script: "for i in {1..40}; do ( " + big + " ); done; echo DONE\n",
			want:   "DONE\n",
		},
		"parent storage is visible to every stage": {
			script: big + "\n{ Y=a; echo $Y; } | { Z=b; cat; echo $Z; }\n",
			want:   "a\nb\n",
		},
		"thousands of variables": {
			script: "for i in {1..5000}; do read v$i <<E\nval\nE\ndone; echo $v1 $v5000\n",
			want:   "val val\n",
		},
		"moderate unquoted splitting": {
			script: "x='" + words + "'\nfor w in $x $x; do echo $w; done | wc -l\n",
			want:   "4000\n",
		},
		"empty unquoted word at the argument cap": {
			script: "echo {1..16384} $empty | wc -w\n",
			want:   "16384\n",
		},
		"concatenated parts at the argument cap": {
			script: "x=b; echo {1..16383} a$x | wc -w\n",
			want:   "16384\n",
		},
		"substitution after a literal at the argument cap": {
			script: "echo {1..16383} a$(echo b) | wc -w\n",
			want:   "16384\n",
		},
		"literal after a substitution at the argument cap": {
			script: "echo {1..16383} $(echo b)a | wc -w\n",
			want:   "16384\n",
		},
		"adjacent substitutions at the argument cap": {
			script: "echo {1..16383} $(echo b)$(echo c) | wc -w\n",
			want:   "16384\n",
		},
		"empty substitution between literals at the argument cap": {
			script: "echo {1..16383} a$(echo)b | wc -w\n",
			want:   "16384\n",
		},
		"trimmed substitution output at the argument cap": {
			script: "IFS=,; echo {1..16384} $(echo) | wc -w\n",
			want:   "16384\n",
		},
		"concatenated parts with split values at the argument cap": {
			script: "x='b '; y=' c'; echo {1..16381} a$x\"q\"$y$x | wc -w\n",
			want:   "16384\n",
		},
		"custom IFS splitting": {
			script: "IFS=,; x='a,b,c d'; for w in $x; do echo \"[$w]\"; done\n",
			want:   "[a]\n[b]\n[c d]\n",
		},
		"unquoted command substitution": {
			script: "for w in $(echo a b c) x{1,2}$(echo d e); do echo $w; done\n",
			want:   "a\nb\nc\nx1d\ne\nx2d\ne\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			stdout, _ := runMeasured(t, tc.script, nil, &stderr,
				interp.AllowedCommands([]string{"rshell:echo", "rshell:cat", "rshell:read", "rshell:wc"}))
			assert.Equal(t, tc.want, stdout, "stderr: %s", stderr.String())
			assert.Empty(t, stderr.String())
		})
	}
}

// TestVULN1675StorageChargedOnceAndReleasedPromptly pins the shared budget's
// bookkeeping: storage an ancestor still holds is not refunded by hiding or
// replacing it, temporary inline-assignment overlays release their charge,
// and a finished pipeline producer releases its storage before the consumer
// sees EOF.
func TestVULN1675StorageChargedOnceAndReleasedPromptly(t *testing.T) {
	env := interp.Env("PAYLOAD="+strings.Repeat("p", 600<<10), "OTHER="+strings.Repeat("o", 600<<10))
	tests := map[string]struct {
		script string
		want   string
		stderr string
	}{
		"subshell clear then create": {
			script: "A=$PAYLOAD\n( A=; B=$OTHER; echo LEAK )\necho DONE\n",
			want:   "DONE\n",
			stderr: "variable storage limit exceeded",
		},
		"subshell replacement": {
			script: "A=$PAYLOAD\n( A=$OTHER; echo LEAK )\necho DONE\n",
			want:   "DONE\n",
			stderr: "variable storage limit exceeded",
		},
		"pipeline stage clear then create": {
			script: "A=$PAYLOAD\necho | { A=; B=$OTHER; echo LEAK; }\necho DONE\n",
			want:   "DONE\n",
			stderr: "variable storage limit exceeded",
		},
		"inline assignment charged once": {
			script: "X=$PAYLOAD true\necho OK\n",
			want:   "OK\n",
		},
		"repeated inline assignments release their overlay": {
			script: "for i in 1 2 3 4 5 6 7 8; do X=$PAYLOAD Y=x$i true; done\necho OK\n",
			want:   "OK\n",
		},
		"restoring an inherited temporary assignment": {
			script: "A=$PAYLOAD\n( A=x true; B=y; echo OK )\necho DONE\n",
			want:   "OK\nDONE\n",
		},
		"finished producer releases before consumer EOF": {
			script: "{ X=$PAYLOAD; echo marker; } | { cat >/dev/null; Y=$PAYLOAD; echo OK; }\n",
			want:   "OK\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			stdout, _ := runMeasured(t, tc.script, nil, &stderr, env,
				interp.AllowedCommands([]string{"rshell:echo", "rshell:cat", "rshell:true"}))
			assert.Equal(t, tc.want, stdout, "stderr: %s", stderr.String())
			if tc.stderr == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), tc.stderr)
			}
		})
	}
}
