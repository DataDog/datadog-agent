// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
)

// captureTransport is an http.RoundTripper that records every request body it
// sees and returns a 200 response without making a real network call. It is
// used to intercept payloads the installer telemetry sender would otherwise
// POST to intake.
type captureTransport struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		c.mu.Lock()
		c.bodies = append(c.bodies, b)
		c.mu.Unlock()
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     http.Header{},
		Request:    req,
	}, nil
}

// newCapturingTelemetry builds a Telemetry instance whose HTTP client points
// at an in-memory RoundTripper, so tests can inspect the payload that would
// otherwise be POSTed to intake without making a real network call.
func newCapturingTelemetry(t *testing.T) (*telemetry.Telemetry, *captureTransport) {
	t.Helper()
	// Interpreter tests share one process and collectively execute enough
	// commands to exhaust the production lifetime limit. Each telemetry test
	// needs an isolated budget so it can assert the spans from its own run.
	telemetrySpansStarted.Store(0)
	ct := &captureTransport{}
	tel := telemetry.NewTelemetry(
		&http.Client{Transport: ct},
		"test-api-key",
		"datadoghq.com",
		"rshell-test",
	)
	return tel, ct
}

// TestRunEmitsTracerSpan verifies that Run creates a "run" span tagged with
// the rshell version, exit code, and a "success" outcome for a clean run.
func TestRunEmitsTracerSpan(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "true"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	runSpan := findOneSpanByResource(spans, "run")
	require.NotNil(t, runSpan, "expected a run span")
	assert.Equal(t, version.Version, runSpan.Meta["rshell.version"])
	assert.Equal(t, "success", runSpan.Meta["rshell.run.outcome"])
	assert.Equal(t, float64(0), runSpan.Metrics["rshell.run.exit_code"])
	assert.Equal(t, "false", runSpan.Meta["rshell.run.invoked_via_cli"])
}

// TestRunSpanInvokedViaCLI verifies that [InvokedViaCLI] is reported on the
// run span even when [DisableDetailedTelemetry] suppresses the command and
// options tags, since it is invocation metadata rather than command content.
func TestRunSpanInvokedViaCLI(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), InvokedViaCLI(), DisableDetailedTelemetry())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "true"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	runSpan := findOneSpanByResource(spans, "run")
	require.NotNil(t, runSpan, "expected a run span")
	assert.Equal(t, "true", runSpan.Meta["rshell.run.invoked_via_cli"])
}

// TestRunSpanCommandAndOptions verifies that the run span records the raw
// script text supplied via [Script], plus the effective [RunnerOption]
// configuration (mode, timeout, proc path, host prefix, allowed paths,
// allowed commands, elevatable commands, and allowed system services).
func TestRunSpanCommandAndOptions(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	dir := t.TempDir()
	script := "echo hi"
	r, err := New(
		Script(script),
		AllowedPaths([]string{dir + ":rw"}),
		AllowedCommands([]string{"rshell:echo"}),
		SelectiveElevation([]string{"rshell:echo"}, func(_ context.Context, _ string, run func()) error {
			run()
			return nil
		}),
		AllowedSystemServices([]SystemServiceControlGrant{
			{Service: "foo.service", Actions: []SystemServiceAction{SystemServiceRead}},
		}),
		ProcPath("/custom/proc"),
		HostPrefix(dir),
		MaxExecutionTime(5*time.Second),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, script))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	runSpan := findOneSpanByResource(spans, "run")
	require.NotNil(t, runSpan, "expected a run span")

	assert.Equal(t, script, runSpan.Meta["rshell.run.command"])
	assert.Equal(t, "remediation", runSpan.Meta["rshell.run.options.mode"])
	assert.Equal(t, "5s", runSpan.Meta["rshell.run.options.max_execution_time"])
	assert.Equal(t, "/custom/proc", runSpan.Meta["rshell.run.options.proc_path"])
	assert.Equal(t, dir, runSpan.Meta["rshell.run.options.host_prefix"])
	assert.Equal(t, dir+":rw", runSpan.Meta["rshell.run.options.allowed_paths"])
	assert.Equal(t, "false", runSpan.Meta["rshell.run.options.allow_all_commands"])
	assert.Equal(t, "echo", runSpan.Meta["rshell.run.options.allowed_commands"])
	assert.Equal(t, "echo", runSpan.Meta["rshell.run.options.elevatable_commands"])
	assert.Equal(t, "foo.service:read", runSpan.Meta["rshell.run.options.allowed_system_services"])
	assert.Equal(t, "false", runSpan.Meta["rshell.run.options.systemd_target_configured"])
}

// TestRunSpanDetailedTelemetryDisabled verifies that [DisableDetailedTelemetry]
// suppresses rshell.run.command and every rshell.run.options.* tag, while
// leaving unrelated tags such as rshell.version and rshell.run.exit_code
// intact.
func TestRunSpanDetailedTelemetryDisabled(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	script := "echo hi"
	r, err := New(
		allowAllCommandsOpt(),
		Script(script),
		WithMode(ModeRemediation),
		DisableDetailedTelemetry(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, script))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	runSpan := findOneSpanByResource(spans, "run")
	require.NotNil(t, runSpan, "expected a run span")

	_, hasCommand := runSpan.Meta["rshell.run.command"]
	assert.False(t, hasCommand, "rshell.run.command should be suppressed")
	for tag := range runSpan.Meta {
		assert.NotContains(t, tag, "rshell.run.options.", "no rshell.run.options.* tag should be present")
	}
	assert.NotEmpty(t, runSpan.Meta["rshell.version"])
	assert.Equal(t, float64(0), runSpan.Metrics["rshell.run.exit_code"])
}

// TestRunSpanOutcome verifies the outcome classification on the run span:
// any script completion (zero exit, non-zero exit, explicit exit N, or
// scripts that hit blocked/unknown commands along the way) is "success"
// — rshell treats these as the shell doing its job. The exit code lands
// in rshell.run.exit_code; blocked/unknown dispatches are counted via
// rshell.run.unallowed_count and rshell.run.unknown_count.
func TestRunSpanOutcome(t *testing.T) {
	cases := []struct {
		name     string
		script   string
		opts     []RunnerOption
		outcome  string
		exitCode float64
	}{
		{"natural exit 0", "true",
			[]RunnerOption{allowAllCommandsOpt()},
			"success", 0},
		{"explicit exit 0", "exit 0",
			[]RunnerOption{allowAllCommandsOpt()},
			"success", 0},
		{"last command returns non-zero", "false",
			[]RunnerOption{allowAllCommandsOpt()},
			"success", 1},
		{"explicit exit N", "exit 7",
			[]RunnerOption{allowAllCommandsOpt()},
			"success", 7},
		{"blocked command on the way", "cat /etc/hostname; echo ok",
			[]RunnerOption{AllowedCommands([]string{"rshell:echo"}), StdIO(nil, io.Discard, io.Discard)},
			"success", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tel, ct := newCapturingTelemetry(t)
			r, err := New(tc.opts...)
			require.NoError(t, err)
			t.Cleanup(func() { r.Close() })

			traceID := newTestTraceID()
			_ = runWithTracedContext(t, r, traceID, tc.script)
			tel.Stop()

			spans := ct.spansForTrace(t, traceID)
			runSpan := findOneSpanByResource(spans, "run")
			require.NotNil(t, runSpan)
			assert.Equal(t, tc.outcome, runSpan.Meta["rshell.run.outcome"])
			assert.Equal(t, tc.exitCode, runSpan.Metrics["rshell.run.exit_code"])
			assert.Empty(t, runSpan.Meta["error.message"],
				"script completion should not flag the run span as errored")
		})
	}
}

// TestRunSpanPolicyCounters verifies the three per-run counters on the
// run span — dispatched_count (commands that actually ran a builtin),
// unallowed_count (blocked by AllowedCommands), and unknown_count
// (missing from the builtin registry) — tally correctly, including
// across pipeline stages and for commands that are both blocked and
// unknown (which bump both counters since is_allowed and is_known are
// independent facts about the command name).
func TestRunSpanPolicyCounters(t *testing.T) {
	cases := []struct {
		name            string
		script          string
		opts            []RunnerOption
		totalCount      float64
		dispatchedCount float64
		unallowedCount  float64
		unknownCount    float64
	}{
		{"no rejections", "echo a; echo b",
			[]RunnerOption{allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard)},
			2, 2, 0, 0},
		{"one blocked, one dispatched", "echo a; cat /etc/hostname",
			[]RunnerOption{AllowedCommands([]string{"rshell:echo"}), StdIO(nil, io.Discard, io.Discard)},
			2, 1, 1, 0},
		{"blocked builtins across pipeline stages",
			"cat x; cat y | grep z",
			[]RunnerOption{AllowedCommands([]string{"rshell:echo"}), StdIO(nil, io.Discard, io.Discard)},
			3, 0, 3, 0},
		{"one unknown", "echo a; nosuchcmd_xyz",
			[]RunnerOption{allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard)},
			2, 1, 0, 1},
		{"blocked and unknown simultaneously (pwd-like)",
			"echo a; pwd_no_such",
			[]RunnerOption{AllowedCommands([]string{"rshell:echo"}), StdIO(nil, io.Discard, io.Discard)},
			2, 1, 1, 1},
		{"blocked builtin plus allowed-but-unknown",
			"cat x; totally_made_up",
			[]RunnerOption{AllowedCommands([]string{"rshell:totally_made_up"}), StdIO(nil, io.Discard, io.Discard)},
			2, 0, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tel, ct := newCapturingTelemetry(t)
			r, err := New(tc.opts...)
			require.NoError(t, err)
			t.Cleanup(func() { r.Close() })

			traceID := newTestTraceID()
			_ = runWithTracedContext(t, r, traceID, tc.script)
			tel.Stop()

			spans := ct.spansForTrace(t, traceID)
			runSpan := findOneSpanByResource(spans, "run")
			require.NotNil(t, runSpan)
			assert.Equal(t, tc.totalCount, runSpan.Metrics["rshell.run.commands.total"])
			assert.Equal(t, tc.dispatchedCount, runSpan.Metrics["rshell.run.commands.dispatched"])
			assert.Equal(t, tc.unallowedCount, runSpan.Metrics["rshell.run.commands.unallowed"])
			assert.Equal(t, tc.unknownCount, runSpan.Metrics["rshell.run.commands.unknown"])
			// These are accounting tags, not error signals.
			assert.Equal(t, "success", runSpan.Meta["rshell.run.outcome"])
			assert.Empty(t, runSpan.Meta["error.message"])
		})
	}
}

// decodedEvent mirrors the top-level JSON shape the installer telemetry sender
// POSTs to intake. Only the fields needed by tests are listed.
type decodedEvent struct {
	RequestType string          `json:"request_type"`
	Payload     json.RawMessage `json:"payload"`
}

type decodedTracePayload struct {
	Traces [][]decodedSpan `json:"traces"`
}

type decodedSpan struct {
	Name     string             `json:"name"`
	Resource string             `json:"resource"`
	TraceID  uint64             `json:"trace_id"`
	SpanID   uint64             `json:"span_id"`
	ParentID uint64             `json:"parent_id"`
	Meta     map[string]string  `json:"meta"`
	Metrics  map[string]float64 `json:"metrics"`
}

// spansForTrace decodes every captured request body and returns the spans that
// belong to the given trace ID. Filtering by trace ID isolates spans produced
// by a single test from any leftover spans that accumulated on the
// package-level global tracer across other tests.
func (c *captureTransport) spansForTrace(t *testing.T, traceID uint64) []decodedSpan {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var spans []decodedSpan
	for _, body := range c.bodies {
		var evt decodedEvent
		require.NoError(t, json.Unmarshal(body, &evt))
		if evt.RequestType != "traces" {
			continue
		}
		var pl decodedTracePayload
		require.NoError(t, json.Unmarshal(evt.Payload, &pl))
		for _, trace := range pl.Traces {
			for _, sp := range trace {
				if sp.TraceID == traceID {
					spans = append(spans, sp)
				}
			}
		}
	}
	return spans
}

// findSpanByCommand looks up a command span (operation "command") whose
// resource matches cmdName. The rshell.command.name tag is cross-checked to
// guard against a non-command span having a coincidentally matching resource.
func findSpanByCommand(spans []decodedSpan, cmdName string) *decodedSpan {
	for i := range spans {
		if spans[i].Name == "command" && spans[i].Resource == cmdName &&
			spans[i].Meta["rshell.command.name"] == cmdName {
			return &spans[i]
		}
	}
	return nil
}

// findSpansByResource returns every span whose resource equals the given
// value. For control_flow spans (if/for/pipeline/for.iteration) and the run
// span, resource is the canonical identity of the span kind — operation name
// is shared across kinds (e.g. all control_flow spans share operation
// "control_flow").
func findSpansByResource(spans []decodedSpan, resource string) []decodedSpan {
	var out []decodedSpan
	for _, sp := range spans {
		if sp.Resource == resource {
			out = append(out, sp)
		}
	}
	return out
}

func findOneSpanByResource(spans []decodedSpan, resource string) *decodedSpan {
	matches := findSpansByResource(spans, resource)
	if len(matches) == 0 {
		return nil
	}
	return &matches[0]
}

// testTraceIDCounter gives each test a unique, non-zero trace ID so the
// captured payload can be filtered regardless of what else accumulated on the
// shared global tracer before the test ran.
var testTraceIDCounter atomic.Uint64

func newTestTraceID() uint64 { return testTraceIDCounter.Add(1) + 1 }

// runWithTracedContext runs prog on r under a context that carries a parent
// span with the given trace ID, then finishes the parent. Any rshell spans
// created during the run inherit the trace ID, which is what the test filters
// on.
func runWithTracedContext(t *testing.T, r *Runner, traceID uint64, script string) error {
	t.Helper()
	parent, ctx := telemetry.StartSpanFromIDs(context.Background(), "test.parent",
		uint64ToDec(traceID), "0")
	err := r.Run(ctx, parseScript(t, script))
	parent.Finish(nil)
	return err
}

func uint64ToDec(v uint64) string {
	// strconv would work too — small helper to avoid the extra import.
	buf := make([]byte, 0, 20)
	if v == 0 {
		return "0"
	}
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	return string(buf)
}

// TestCallEmitsCommandSpan verifies that invoking a command emits an
// "command" span tagged with the command name.
func TestCallEmitsCommandSpan(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "true"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "true")
	require.NotNil(t, cmd, "expected a command span for 'true'")
	assert.Equal(t, "true", cmd.Meta["rshell.command.name"])
	assert.Equal(t, "true", cmd.Meta["rshell.command.is_allowed"])
	assert.Equal(t, "true", cmd.Meta["rshell.command.is_known"])
	assert.Equal(t, "false", cmd.Meta["rshell.command.has_stdin_pipe"])
	assert.Equal(t, "false", cmd.Meta["rshell.command.has_output_redirect"])
	assert.Equal(t, float64(0), cmd.Metrics["rshell.command.argc"])
	assert.Equal(t, float64(0), cmd.Metrics["rshell.command.exit_code"])
}

// TestCommandSpanArgc verifies argc reflects the number of arguments passed
// (not counting argv[0]).
func TestCommandSpanArgc(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo a b c"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "echo")
	require.NotNil(t, cmd)
	assert.Equal(t, float64(3), cmd.Metrics["rshell.command.argc"])
}

// TestCommandSpanFlags verifies that flag-shaped arguments are captured on
// the span as a leading/trailing-comma-padded list, with glued long-form
// values stripped and positional (non-flag) arguments excluded. The padding
// lets a dashboard query for one exact flag (e.g. "*,-n,*") match without
// false-positiving on a longer flag containing the same substring.
func TestCommandSpanFlags(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo -n --file=secret.txt arg1"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "echo")
	require.NotNil(t, cmd)
	assert.Equal(t, ",-n,--file,", cmd.Meta["rshell.command.flags"])
}

// TestCommandSpanFlagsNone verifies that the flags tag is omitted entirely
// when no arguments look like flags.
func TestCommandSpanFlagsNone(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo a b c"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "echo")
	require.NotNil(t, cmd)
	_, ok := cmd.Meta["rshell.command.flags"]
	assert.False(t, ok)
}

// TestCommandSpanFlagsPaddingAvoidsSubstringCollision verifies that a
// dashboard-style wildcard query anchored on both sides ("*,-n,*") matches
// a call carrying "-n" but not a call carrying only a longer flag that
// happens to contain "-n" as a substring, thanks to the leading/trailing
// comma padding.
func TestCommandSpanFlagsPaddingAvoidsSubstringCollision(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo --filename x"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "echo")
	require.NotNil(t, cmd)
	flags := cmd.Meta["rshell.command.flags"]
	assert.Equal(t, ",--filename,", flags)
	assert.NotContains(t, flags, ",-n,", "a bare -n query must not match a call that only used --filename")
}

// TestCommandFlagsNoValueLeak is a table-driven test over commandFlags
// directly (rather than through a full span) covering every way a flag's
// value can be attached to it, to make sure the value itself is never
// captured — only the flag name.
func TestCommandFlagsNoValueLeak(t *testing.T) {
	const secret = "s3cr3t"

	tests := []struct {
		name string
		args []string
		want []string
	}{
		// 1. Short flag, boolean-style, no value at all.
		{"short flag alone", []string{"-r"}, []string{"-r"}},
		// 1. Short flag with its value as a separate, space-delimited arg.
		{"short flag with space value", []string{"-r", secret}, []string{"-r"}},
		// 1. Short flag with its value glued on via "=".
		{"short flag with equals value", []string{"-r=" + secret}, []string{"-r"}},
		// 3. Short flag with its value glued on directly, no separator —
		// the value has no delimiter to strip at, so only the flag letter
		// is kept.
		{"short flag with glued value", []string{"-r" + secret}, []string{"-r"}},
		// 3. Combined short boolean cluster: indistinguishable from a
		// glued value, so only the first flag letter is recorded.
		{"combined short flags", []string{"-la"}, []string{"-l"}},
		// 2. Long flag, boolean-style, no value at all.
		{"long flag alone", []string{"--dry-run"}, []string{"--dry-run"}},
		// 2. Long flag with its value as a separate, space-delimited arg.
		{"long flag with space value", []string{"--dry-run", secret}, []string{"--dry-run"}},
		// 2. Long flag with its value glued on via "=".
		{"long flag with equals value", []string{"--dry-run=" + secret}, []string{"--dry-run"}},
		// 3. "--" ends flag parsing; nothing after it is captured even if
		// flag-shaped, so a positional value can't be smuggled through.
		{"end of options terminator", []string{"--", "-r", secret}, nil},
		// Lone "-" is a conventional stdin/stdout marker, not a flag.
		{"lone dash", []string{"-"}, nil},
		// Positional (non-flag) arguments are never captured.
		{"positional args only", []string{"a", "b"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandFlags(tt.args)
			assert.Equal(t, tt.want, got)
			for _, f := range got {
				assert.NotContains(t, f, secret, "captured flag must never contain the flag's value")
			}
		})
	}
}

// TestCommandSpanDisallowed verifies that a command blocked by AllowedCommands
// is captured with is_allowed=false and exit_code=127, and that the is_known
// tag is still populated regardless of the short-circuit.
func TestCommandSpanDisallowed(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	// Allow echo only; cat is a known builtin but not allowed.
	r, err := New(AllowedCommands([]string{"rshell:echo"}), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	_ = runWithTracedContext(t, r, traceID, "cat /etc/hostname")
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "cat")
	require.NotNil(t, cmd, "expected a command span for the blocked call")
	assert.Equal(t, "false", cmd.Meta["rshell.command.is_allowed"])
	assert.Equal(t, "true", cmd.Meta["rshell.command.is_known"])
	assert.Equal(t, float64(127), cmd.Metrics["rshell.command.exit_code"])
}

// TestCommandSpanUnknown verifies that a command not in the builtin registry
// is captured with is_known=false, and (under --allow-all-commands) also
// carries is_allowed=true with exit_code=127 from the noExecHandler.
func TestCommandSpanUnknown(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt(), StdIO(nil, io.Discard, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	_ = runWithTracedContext(t, r, traceID, "definitely_not_a_real_command")
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "definitely_not_a_real_command")
	require.NotNil(t, cmd)
	assert.Equal(t, "true", cmd.Meta["rshell.command.is_allowed"])
	assert.Equal(t, "false", cmd.Meta["rshell.command.is_known"])
	assert.Equal(t, float64(127), cmd.Metrics["rshell.command.exit_code"])
}

// TestCommandSpanNonZeroExit verifies the exit_code tag captures the actual
// exit status returned by a builtin (here, the "false" builtin returns 1).
func TestCommandSpanNonZeroExit(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	_ = runWithTracedContext(t, r, traceID, "false")
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	cmd := findSpanByCommand(spans, "false")
	require.NotNil(t, cmd)
	assert.Equal(t, float64(1), cmd.Metrics["rshell.command.exit_code"])
}

// TestCommandSpanPipeline verifies that the left stage of a pipeline has
// has_output_redirect=true (its stdout points at the pipe write end) while the
// right stage has has_stdin_pipe=true (its stdin points at the pipe read end).
func TestCommandSpanPipeline(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	var outBuf bytes.Buffer
	r, err := New(allowAllCommandsOpt(), StdIO(nil, &outBuf, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo hi | cat"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	echo := findSpanByCommand(spans, "echo")
	cat := findSpanByCommand(spans, "cat")
	require.NotNil(t, echo, "expected echo span")
	require.NotNil(t, cat, "expected cat span")

	assert.Equal(t, "false", echo.Meta["rshell.command.has_stdin_pipe"])
	assert.Equal(t, "true", echo.Meta["rshell.command.has_output_redirect"])
	assert.Equal(t, "true", cat.Meta["rshell.command.has_stdin_pipe"])
	assert.Equal(t, "false", cat.Meta["rshell.command.has_output_redirect"])
}

// TestPipelineSpan verifies that a 3-stage pipeline produces a single
// flattened pipeline span with stage_count=3 and exit_code from the
// rightmost stage.
func TestPipelineSpan(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	var outBuf bytes.Buffer
	r, err := New(allowAllCommandsOpt(), StdIO(nil, &outBuf, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "echo hi | cat | cat"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	pipeSpans := findSpansByResource(spans, "pipeline")
	require.Len(t, pipeSpans, 1, "expected exactly one pipeline span (flattened)")
	assert.Equal(t, float64(3), pipeSpans[0].Metrics["rshell.pipeline.stage_count"])
	assert.Equal(t, float64(0), pipeSpans[0].Metrics["rshell.pipeline.exit_code"])
}

// TestIfSpanBranchTaken covers each possible branch_taken value on a single
// chain shape (if/elif/else): primary, elif, else, and none.
func TestIfSpanBranchTaken(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		count   int     // expected branch_count
		taken   float64 // expected branch_taken
		hasElse bool
	}{
		{"primary", "if true; then true; elif true; then true; else true; fi", 3, 0, true},
		{"elif", "if false; then true; elif true; then true; else true; fi", 3, 1, true},
		{"else", "if false; then true; elif false; then true; else true; fi", 3, -1, true},
		{"none", "if false; then true; elif false; then true; fi", 2, -2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tel, ct := newCapturingTelemetry(t)
			r, err := New(allowAllCommandsOpt())
			require.NoError(t, err)
			t.Cleanup(func() { r.Close() })

			traceID := newTestTraceID()
			require.NoError(t, runWithTracedContext(t, r, traceID, tc.script))
			tel.Stop()

			spans := ct.spansForTrace(t, traceID)
			ifSpan := findOneSpanByResource(spans, "if")
			require.NotNil(t, ifSpan, "expected one if span")
			assert.Equal(t, float64(tc.count), ifSpan.Metrics["rshell.if.branch_count"])
			assert.Equal(t, tc.taken, ifSpan.Metrics["rshell.if.branch_taken"])
		})
	}
}

// TestForSpan verifies for-span tags and that each iteration emits a
// rshell.for.iteration span with the expected zero-based index.
func TestForSpan(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID, "for x in a b c; do true; done"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	forSpan := findOneSpanByResource(spans, "for")
	require.NotNil(t, forSpan)
	assert.Equal(t, "x", forSpan.Meta["rshell.for.variable_name"])
	assert.Equal(t, float64(3), forSpan.Metrics["rshell.for.iteration_count"])
	assert.Equal(t, "false", forSpan.Meta["rshell.for.broke_early"])

	iters := findSpansByResource(spans, "for.iteration")
	require.Len(t, iters, 3)
	indexes := make(map[float64]bool)
	for _, it := range iters {
		indexes[it.Metrics["rshell.for.iteration.index"]] = true
	}
	assert.True(t, indexes[0] && indexes[1] && indexes[2],
		"expected iteration indexes 0, 1, 2; got %v", indexes)
}

// TestForSpanBrokeEarly verifies broke_early is true when the loop exits via
// an explicit break, and iteration_count reflects how many iterations ran
// before the break.
func TestForSpanBrokeEarly(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	r, err := New(allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	require.NoError(t, runWithTracedContext(t, r, traceID,
		"for x in a b c d; do if [ \"$x\" = b ]; then break; fi; done"))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)
	forSpan := findOneSpanByResource(spans, "for")
	require.NotNil(t, forSpan)
	assert.Equal(t, "true", forSpan.Meta["rshell.for.broke_early"])
	// Two iterations ran: x=a (body continues) and x=b (break taken).
	assert.Equal(t, float64(2), forSpan.Metrics["rshell.for.iteration_count"])
}

// TestNesting verifies that for → if → pipeline produces nested spans linked
// via ParentID, so the trace view reflects the control-flow structure.
func TestNesting(t *testing.T) {
	tel, ct := newCapturingTelemetry(t)

	var outBuf bytes.Buffer
	r, err := New(allowAllCommandsOpt(), StdIO(nil, &outBuf, io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	traceID := newTestTraceID()
	script := "for i in a; do if true; then echo hi | cat; fi; done"
	require.NoError(t, runWithTracedContext(t, r, traceID, script))
	tel.Stop()

	spans := ct.spansForTrace(t, traceID)

	forSpan := findOneSpanByResource(spans, "for")
	iterSpan := findOneSpanByResource(spans, "for.iteration")
	ifSpan := findOneSpanByResource(spans, "if")
	pipeSpan := findOneSpanByResource(spans, "pipeline")
	echoSpan := findSpanByCommand(spans, "echo")
	catSpan := findSpanByCommand(spans, "cat")

	require.NotNil(t, forSpan, "expected for span")
	require.NotNil(t, iterSpan, "expected for.iteration span")
	require.NotNil(t, ifSpan, "expected if span")
	require.NotNil(t, pipeSpan, "expected pipeline span")
	require.NotNil(t, echoSpan, "expected echo command span")
	require.NotNil(t, catSpan, "expected cat command span")

	// Walk the expected parent chain: echo/cat → pipeline → if → iteration → for.
	assert.Equal(t, pipeSpan.SpanID, echoSpan.ParentID, "echo's parent should be the pipeline span")
	assert.Equal(t, pipeSpan.SpanID, catSpan.ParentID, "cat's parent should be the pipeline span")
	assert.Equal(t, ifSpan.SpanID, pipeSpan.ParentID, "pipeline's parent should be the if span")
	assert.Equal(t, iterSpan.SpanID, ifSpan.ParentID, "if's parent should be the for.iteration span")
	assert.Equal(t, forSpan.SpanID, iterSpan.ParentID, "for.iteration's parent should be the for span")
}
