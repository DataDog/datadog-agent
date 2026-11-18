// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// runNested runs script with every command allowed and dir readable, and
// returns the exit code, stdout and stderr.
func runNested(t *testing.T, dir, script string) (uint8, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir
	_ = r.Run(context.Background(), parseScript(t, script))
	return r.exit.code, stdout.String(), stderr.String()
}

// xargsChain returns n space-separated "xargs" words. With empty input each
// xargs runs the rest of the line once, so prefixing a command with the chain
// runs it n levels deep.
func xargsChain(n int) string {
	return strings.TrimSpace(strings.Repeat("xargs ", n))
}

func TestNestedCommandDepthLimit(t *testing.T) {
	limitMsg := fmt.Sprintf("nested command depth exceeds maximum of %d", MaxNestedCommandDepth)

	tests := []struct {
		name   string
		script string
		ok     bool
		// The refusal reaches find only as the innermost xargs's exit
		// status, which find treats as a false predicate, so find exits 0.
		findTop bool
	}{
		{
			// The top-level xargs runs directly; each following word is one
			// nested level, ending with echo at MaxNestedCommandDepth.
			name:   "xargs chain at limit",
			script: xargsChain(MaxNestedCommandDepth) + " echo ok",
			ok:     true,
		},
		{
			name:   "xargs chain over limit",
			script: xargsChain(MaxNestedCommandDepth+1) + " echo ok",
		},
		{
			// find -exec and xargs share one depth count.
			name:   "find exec then xargs at limit",
			script: "find . -maxdepth 0 -exec " + xargsChain(MaxNestedCommandDepth-1) + " echo ok ';'",
			ok:     true,
		},
		{
			name:    "find exec then xargs over limit",
			script:  "find . -maxdepth 0 -exec " + xargsChain(MaxNestedCommandDepth) + " echo ok ';'",
			findTop: true,
		},
		{
			name:   "xargs then find exec over limit",
			script: xargsChain(MaxNestedCommandDepth) + " find . -maxdepth 0 -exec echo ok ';'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runNested(t, t.TempDir(), tc.script)
			if tc.ok {
				assert.Equal(t, uint8(0), code, "stderr: %s", stderr)
				assert.Equal(t, "ok\n", stdout)
				assert.Empty(t, stderr)
				return
			}
			if !tc.findTop {
				assert.NotEqual(t, uint8(0), code)
			}
			assert.Empty(t, stdout, "the command past the limit must not run")
			assert.Contains(t, stderr, limitMsg)
		})
	}
}

// TestNestedCommandDepthPerTopLevelCommand checks that depth is counted from
// each top-level command, so sequential and piped commands each get the full
// allowance.
func TestNestedCommandDepthPerTopLevelCommand(t *testing.T) {
	chain := xargsChain(MaxNestedCommandDepth) + " echo ok"
	code, stdout, stderr := runNested(t, t.TempDir(), chain+"; "+chain+"; echo | "+chain)
	assert.Equal(t, uint8(0), code, "stderr: %s", stderr)
	assert.Equal(t, "ok\nok\nok\n", stdout)
	assert.Empty(t, stderr)
}

var nestedDepthTestSeq atomic.Int64

// TestNestedCommandDepthSelfDispatch registers a builtin that runs itself
// again on every call, alternating RunCommand and RunCommandWithStdin and
// passing a fresh context each time, and checks the runner stops it at the
// limit instead of letting the recursion grow until it stops itself.
func TestNestedCommandDepthSelfDispatch(t *testing.T) {
	registerBuiltins()
	name := fmt.Sprintf("interptestselfdispatch%d", nestedDepthTestSeq.Add(1))
	// Ends the recursion if the runner does not, so the test fails on an
	// assertion rather than exhausting the stack.
	const ownCap = 10 * MaxNestedCommandDepth

	var calls int
	var refusal error
	builtins.Command{
		Name:        name,
		Description: "test-only builtin that runs itself again",
		MakeFlags: builtins.NoFlags(func(_ context.Context, callCtx *builtins.CallContext, _ []string) builtins.Result {
			calls++
			if calls >= ownCap {
				return builtins.Result{}
			}
			var code uint8
			var err error
			if calls%2 == 0 {
				code, err = callCtx.RunCommandWithStdin(context.Background(), "", name, nil, strings.NewReader(""))
			} else {
				code, err = callCtx.RunCommand(context.Background(), "", name, nil)
			}
			if err != nil {
				refusal = err
				return builtins.Result{Code: 1}
			}
			return builtins.Result{Code: code}
		}),
	}.Register()

	code, stdout, stderr := runNested(t, t.TempDir(), name)

	// One top-level call plus one per nested level.
	assert.Equal(t, MaxNestedCommandDepth+1, calls)
	require.Error(t, refusal)
	assert.Contains(t, refusal.Error(), fmt.Sprintf("nested command depth exceeds maximum of %d", MaxNestedCommandDepth))
	assert.Equal(t, uint8(1), code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}
