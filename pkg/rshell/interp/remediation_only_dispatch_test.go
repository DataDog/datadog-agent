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

// newReadOnlyRunner creates a runner in the default read-only mode with dir
// configured read-write in AllowedPaths and every command allowed. The
// read-write root is deliberate: it proves the refusal comes from the mode
// gate, not from a missing sandbox root.
func newReadOnlyRunner(t *testing.T, dir string) (*Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir + ":rw"}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir
	return r, &stdout, &stderr
}

// remediationOnlyNames returns every registered builtin whose metadata marks
// it remediation-only.
func remediationOnlyNames(t *testing.T) []string {
	t.Helper()
	// Builtin registration is lazy (registerOnce, driven by New); force it so
	// the registry is populated even if no runner has been built yet.
	registerBuiltins()
	var names []string
	for _, name := range builtins.Names() {
		meta, ok := builtins.Meta(name)
		require.True(t, ok, "no metadata registered for builtin %q", name)
		if meta.RemediationOnly {
			names = append(names, name)
		}
	}
	require.NotEmpty(t, names, "expected at least one remediation-only builtin")
	return names
}

// TestRemediationOnlyBuiltinsRefusedInReadOnlyMode iterates the builtin
// registry and asserts that every command flagged RemediationOnly is refused
// when the shell runs in read-only mode. This is what makes the flag
// load-bearing: a future remediation-only builtin that forgets its own
// capability check is still refused by the dispatch gate, and this test fails
// if that gate ever stops firing.
func TestRemediationOnlyBuiltinsRefusedInReadOnlyMode(t *testing.T) {
	dir := t.TempDir()
	for _, name := range remediationOnlyNames(t) {
		t.Run(name, func(t *testing.T) {
			// Bare invocation and --help must behave identically: the
			// refusal happens before flag parsing, so no usage text leaks.
			for _, script := range []string{name, name + " --help"} {
				r, stdout, stderr := newReadOnlyRunner(t, dir)
				// A non-zero exit surfaces as an error from Run; the exit
				// code itself is asserted below.
				_ = r.Run(context.Background(), parseScript(t, script))

				assert.Equal(t, uint8(1), r.exit.code, "script %q should exit 1", script)
				assert.Empty(t, stdout.String(), "script %q must not print help or other stdout", script)
				assert.NotEmpty(t, stderr.String(), "script %q must explain the refusal on stderr", script)
				assert.True(t, strings.HasPrefix(stderr.String(), name+": "),
					"script %q stderr %q should be prefixed with the command name", script, stderr.String())
				assert.Contains(t, strings.ToLower(stderr.String()), "remediation mode",
					"script %q stderr %q should point at remediation mode", script, stderr.String())
			}
		})
	}
}

// TestRemediationOnlyRefusalUsesRegisteredMessage verifies the dispatch gate
// reproduces each command's own refusal wording byte-for-byte, so adding the
// gate did not change any user-visible message.
func TestRemediationOnlyRefusalUsesRegisteredMessage(t *testing.T) {
	dir := t.TempDir()
	for _, name := range remediationOnlyNames(t) {
		meta, ok := builtins.Meta(name)
		require.True(t, ok)
		require.NotEmpty(t, meta.RemediationDeniedMessage,
			"remediation-only builtin %q must carry a refusal message", name)

		r, _, stderr := newReadOnlyRunner(t, dir)
		_ = r.Run(context.Background(), parseScript(t, name))
		assert.Equal(t, meta.RemediationDeniedMessage, stderr.String())
	}
}

// TestRemediationOnlyBuiltinsAllowedInRemediationMode is the negative control:
// the gate must not fire when remediation mode is on. The commands may still
// fail for their own reasons (missing operands, unavailable systemd), but they
// must not be refused with the read-only message.
func TestRemediationOnlyBuiltinsAllowedInRemediationMode(t *testing.T) {
	dir := t.TempDir()
	for _, name := range remediationOnlyNames(t) {
		t.Run(name, func(t *testing.T) {
			meta, ok := builtins.Meta(name)
			require.True(t, ok)

			r, _, stderr := newRemediationRunner(t, dir)
			_ = r.Run(context.Background(), parseScript(t, name+" --help"))
			assert.NotEqual(t, meta.RemediationDeniedMessage, stderr.String(),
				"%s must not be refused in remediation mode", name)
		})
	}
}

// remediationOnlyGateTestSeq makes each
// TestRemediationOnlyGateCatchesBuiltinWithoutOwnCheck invocation register a
// distinct builtin name, since the process-wide registry panics on a
// duplicate name and never forgets one: without this, running the test more
// than once in the same binary (e.g. `-count=2` or a shuffled/stress rerun)
// would panic on the second registration.
var remediationOnlyGateTestSeq atomic.Int64

// TestRemediationOnlyGateCatchesBuiltinWithoutOwnCheck registers a
// remediation-only builtin that deliberately performs no gating of its own —
// the mistake a future contributor could make — and proves the dispatch gate
// refuses it anyway, before the handler runs.
func TestRemediationOnlyGateCatchesBuiltinWithoutOwnCheck(t *testing.T) {
	registerBuiltins()
	name := fmt.Sprintf("interptestremediationonly%d", remediationOnlyGateTestSeq.Add(1))

	var ran bool
	builtins.Command{
		Name:        name,
		Description: "test-only builtin that forgets its own remediation check",
		MakeFlags: builtins.NoFlags(func(_ context.Context, callCtx *builtins.CallContext, _ []string) builtins.Result {
			ran = true
			callCtx.Out("should never run\n")
			return builtins.Result{}
		}),
		RemediationOnly: true,
	}.Register()

	dir := t.TempDir()
	r, stdout, stderr := newReadOnlyRunner(t, dir)
	_ = r.Run(context.Background(), parseScript(t, name))

	assert.False(t, ran, "handler must not be reached in read-only mode")
	assert.Equal(t, uint8(1), r.exit.code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, builtins.DefaultRemediationDeniedMessage(name), stderr.String())
}

// TestRemediationOnlyGateCatchesHelpDispatch is the `help <name>` counterpart
// to TestRemediationOnlyGateCatchesBuiltinWithoutOwnCheck: help.go looks up
// and invokes a builtin's handler directly (to capture its --help output),
// bypassing the interp dispatch gate entirely. A remediation-only builtin
// that forgets its own check must still be refused when reached this way.
func TestRemediationOnlyGateCatchesHelpDispatch(t *testing.T) {
	registerBuiltins()
	name := fmt.Sprintf("interptestremediationonlyhelp%d", remediationOnlyGateTestSeq.Add(1))

	var ran bool
	builtins.Command{
		Name:        name,
		Description: "test-only builtin that forgets its own remediation check",
		MakeFlags: builtins.NoFlags(func(_ context.Context, callCtx *builtins.CallContext, _ []string) builtins.Result {
			ran = true
			callCtx.Out("should never run\n")
			return builtins.Result{}
		}),
		RemediationOnly: true,
	}.Register()

	dir := t.TempDir()
	r, stdout, stderr := newReadOnlyRunner(t, dir)
	_ = r.Run(context.Background(), parseScript(t, "help "+name))

	assert.False(t, ran, "handler must not be reached via help in read-only mode")
	assert.Empty(t, stdout.String())
	assert.Equal(t, builtins.DefaultRemediationDeniedMessage(name), stderr.String())
}

// TestRemediationDeniedMessageOnlySetForRemediationOnly guards the metadata
// invariant in the other direction: a refusal message on a command that is not
// remediation-only would be dead configuration.
func TestRemediationDeniedMessageOnlySetForRemediationOnly(t *testing.T) {
	for _, name := range builtins.Names() {
		meta, ok := builtins.Meta(name)
		require.True(t, ok)
		if meta.RemediationOnly {
			assert.True(t, strings.HasSuffix(meta.RemediationDeniedMessage, "\n"),
				"%s refusal message must end with a newline", name)
			continue
		}
		assert.Empty(t, meta.RemediationDeniedMessage,
			"%s is not remediation-only but declares a refusal message", name)
	}
}
