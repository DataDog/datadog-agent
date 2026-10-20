// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Blocked-attack regression tests added by the vuln-hunt campaign
// 2026-05-18-initial-audit (target: xargs). Each test pins a hypothesis the
// campaign generated; passing means the corresponding attack is blocked.
package xargs_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H3: -I substitutes REPLSTR into the command name as well as the argument
// vector. An attacker who controls stdin can therefore make the resolved
// command an arbitrary string. The sandbox claim is that CommandAllowed runs
// after resolveCmd, so a substituted-in command that is not in the allowed
// set is still rejected.
func TestVulnHuntBuiltinFlagDrivenExploit_IReplaceCmdNameStillSandboxed(t *testing.T) {
	dir := t.TempDir()
	// Stdin contains the name of a command we want xargs to "promote" to
	// the resolved command. /bin/sh is not a registered builtin. Without
	// the post-substitution CommandAllowed check, xargs would happily call
	// RunCommand("/bin/sh"). With it, xargs must refuse.
	_, stderr, code := runScript(t, "echo /bin/sh | xargs -I@ @", dir,
		interp.AllowedPaths([]string{dir}),
		interp.AllowedCommands([]string{"rshell:xargs", "rshell:echo"}))
	assert.NotEqual(t, 0, code, "substituted command must be rejected, got exit 0; stderr=%q", stderr)
	assert.True(t,
		strings.Contains(stderr, "command not allowed") ||
			strings.Contains(stderr, "unknown command"),
		"stderr should explain the rejection, got %q", stderr)
}

// H6: a stream of pure NUL bytes in default (whitespace) mode forces the
// tokenizer's skipToWhitespace into a long byte-by-byte loop. pollCtx checks
// ctx.Err() every 4096 bytes, so the 30s executor timeout must still fire.
func TestVulnHuntBuiltinResourceExhaustion_InfiniteNullStreamTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow timeout test in -short mode")
	}
	dir := t.TempDir()
	// 8 MiB of NULs is enough that the tokenizer's per-byte loop runs for
	// many polls — under the 30s budget on any reasonable machine the
	// command must terminate (either by completing or by timing out). We
	// bound the goroutine separately so a regression that hangs is caught.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zeros.bin"),
		make([]byte, 8*1024*1024), 0644))

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		runScriptCtx(ctx, t, "xargs -a zeros.bin echo > /dev/null", dir,
			interp.AllowedPaths([]string{dir}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(40 * time.Second):
		t.Fatal("xargs did not honor the 30s timeout on an infinite NUL stream")
	}
}

// H8: -L value validation mirrors -n: 0, negative, and > HardMaxArgs are
// rejected with exit 1 and a "xargs:" error on stderr.
func TestVulnHuntBuiltinIntegerOverflow_LRangeEdges(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		val        string
		shouldFail bool
	}{
		{"0", true},
		{"1", false},
		{"-1", true},
		{"2147483647", true},                // > HardMaxArgs (1 << 20)
		{"9223372036854775807", true},       // > HardMaxArgs
		{"9999999999999999999999999", true}, // overflow at parse
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("L=%s", tc.val), func(t *testing.T) {
			_, stderr, code := cmdRun(t,
				"printf 'a\\nb\\n' | xargs -L "+tc.val+" echo > /dev/null", dir)
			if tc.shouldFail {
				assert.NotEqual(t, 0, code, "expected failure for -L %s, stderr=%q", tc.val, stderr)
				assert.Contains(t, stderr, "xargs:", "expected xargs: prefix, got %q", stderr)
			} else {
				assert.Equal(t, 0, code, "expected success for -L %s, stderr=%q", tc.val, stderr)
			}
		})
	}
}
