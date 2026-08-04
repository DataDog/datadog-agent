// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below are verbatim output from agent-data-plane 1.4.0
// (registry.datadoghq.com/agent-data-plane:1.4.0) run with the preflight mode configuration,
// captured rather than invented. Keep them verbatim: they are the contract this scanner is
// written against, and hand-editing them defeats the point.
const (
	realInfoStartup = `{"timestamp":"2026-07-27T17:57:51.703649Z","level":"INFO","message":"Agent Data Plane starting...","version":"1.4.0","git_hash":"f750b4cc42b959a269c10f7465c78821bec17805","target_arch":"aarch64-unknown-linux-gnu","build_time":"2026-07-15T14:26:44Z","process_id":1,"target":"agent_data_plane::cli::run","filename":"bin/agent-data-plane/src/cli/run.rs","line_number":74}`

	realWarnStandalone = `{"timestamp":"2026-07-27T17:57:51.703871Z","level":"WARN","message":"Running in standalone mode. Origin detection/enrichment and other features dependent upon the Datadog Agent will not be available.","target":"agent_data_plane::internal::env","filename":"bin/agent-data-plane/src/internal/env/mod.rs","line_number":59}`

	// A listener that came up cleanly. Note the nested span objects.
	realInfoListener = `{"timestamp":"2026-07-27T17:57:51.708503Z","level":"INFO","message":"DogStatsD listener started.","listen_addr":"unixgram:///adprun/dsd.socket","target":"saluki_components::sources::dogstatsd","filename":"lib/saluki-components/src/sources/dogstatsd/mod.rs","line_number":1290,"span":{"id":"dsd_in","type":"source","name":"component"},"spans":[{"id":"dsd_in","type":"source","name":"component"}]}`

	// The IPC certificate was missing. ADP renders an anyhow chain into message, so the
	// message field carries embedded newlines while the record stays one physical line.
	realErrorCertMissing = `{"timestamp":"2026-07-27T17:56:28.222521Z","level":"ERROR","message":"Failed to create internal supervisor.\n\nCaused by:\n    Failed to read certificate file '/etc/datadog-agent/ipc_cert.pem' after 20 seconds: No such file or directory (os error 2)","target":"agent_data_plane","filename":"bin/agent-data-plane/src/main.rs","line_number":195}`

	// The DogStatsD socket could not be bound. Five-level anyhow chain.
	realErrorDsdBind = `{"timestamp":"2026-07-27T17:57:16.406247Z","level":"ERROR","message":"Child process 'primary' failed to initialize: Process failed to initialize: Failed to build source 'dsd_in'.\n\nCaused by:\n    0: Process failed to initialize: Failed to build source 'dsd_in'.\n    1: Failed to build source 'dsd_in'.\n    2: Failed to create unixgram listener: failed to configure read/write permissions for listener on address unixgram:///adpwork/dsd.socket: Invalid argument (os error 22)\n    3: failed to configure read/write permissions for listener on address unixgram:///adpwork/dsd.socket: Invalid argument (os error 22)\n    4: Invalid argument (os error 22)","target":"agent_data_plane","filename":"bin/agent-data-plane/src/main.rs","line_number":195}`

	// Same failure with extra structured fields attached.
	realErrorSupervisor = `{"timestamp":"2026-07-27T17:57:16.385952Z","level":"ERROR","message":"Child process failed to initialize: Process failed to initialize: Failed to build source 'dsd_in'.","supervisor_id":"adp-root","worker_name":"primary","target":"saluki_core::runtime::supervisor","filename":"lib/saluki-core/src/runtime/supervisor.rs","line_number":938}`

	realInfoShutdown = `{"timestamp":"2026-07-27T18:05:37.479703Z","level":"INFO","message":"Agent Data Plane shut down successfully.","target":"agent_data_plane::cli::run","filename":"bin/agent-data-plane/src/cli/run.rs","line_number":239}`

	// A bad API key. ADP reports this at WARN, not ERROR, which is why warnings need a
	// finding of their own — this is a hard blocker, not noise.
	realWarnInvalidAPIKey = `{"timestamp":"2026-07-27T19:31:11.910356Z","level":"WARN","message":"Datadog API key is invalid.","endpoint":"https://1-4-0-adp.agent.datadoghq.com/","validation_endpoint":"https://api.datadoghq.com/","target":"saluki_components::common::datadog::validation","filename":"lib/saluki-components/src/common/datadog/validation.rs","line_number":286,"span":{"id":"dd_out","type":"forwarder","name":"component"}}`
)

func TestClassifyLineRealRecords(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantOK     bool
		wantLevel  string
		wantTarget string
		// wantSigContains is checked as a substring of the signature.
		wantSigContains string
	}{
		{
			name:   "INFO is ignored",
			line:   realInfoStartup,
			wantOK: false,
		},
		{
			name:   "INFO with nested spans is ignored",
			line:   realInfoListener,
			wantOK: false,
		},
		{
			name:   "INFO shutdown is ignored",
			line:   realInfoShutdown,
			wantOK: false,
		},
		{
			name:            "WARN standalone mode",
			line:            realWarnStandalone,
			wantOK:          true,
			wantLevel:       levelWarn,
			wantTarget:      "agent_data_plane::internal::env",
			wantSigContains: "Running in standalone mode",
		},
		{
			name:            "ERROR with a multi-line anyhow chain",
			line:            realErrorCertMissing,
			wantOK:          true,
			wantLevel:       levelError,
			wantTarget:      "agent_data_plane",
			wantSigContains: "Failed to create internal supervisor. | | Caused by:",
		},
		{
			name:            "ERROR with extra structured fields",
			line:            realErrorSupervisor,
			wantOK:          true,
			wantLevel:       levelError,
			wantTarget:      "saluki_core::runtime::supervisor",
			wantSigContains: "Failed to build source 'dsd_in'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := classifyLine(tt.line)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.wantLevel, got.Level)
			assert.Equal(t, tt.wantTarget, got.Target)
			assert.Contains(t, got.Signature, tt.wantSigContains)
		})
	}
}

// TestClassifyLineMultiLineMessageStaysOneRecord is the property that justifies forcing
// JSON: ADP renders a whole error chain into a single message field, and with JSON there is
// never any ambiguity about where one log event ends.
func TestClassifyLineMultiLineMessageStaysOneRecord(t *testing.T) {
	got, ok := classifyLine(realErrorDsdBind)
	require.True(t, ok)
	assert.Equal(t, levelError, got.Level)
	assert.NotContains(t, got.Signature, "\n", "the signature must stay on one line")
	// The chain is preserved, because it is the diagnostic part.
	assert.Contains(t, got.Signature, "Failed to create unixgram listener")
	assert.Contains(t, got.Signature, "Invalid argument (os error <n>)")
}

func TestClassifyLineLevels(t *testing.T) {
	for _, tc := range []struct {
		level     string
		wantOK    bool
		wantLevel string
	}{
		{"ERROR", true, levelError},
		{"error", true, levelError},
		{"FATAL", true, levelError},
		{"CRITICAL", true, levelError},
		{"WARN", true, levelWarn},
		{"WARNING", true, levelWarn},
		{"warn", true, levelWarn},
		{"INFO", false, ""},
		{"DEBUG", false, ""},
		{"TRACE", false, ""},
		{"", false, ""},
	} {
		t.Run(tc.level, func(t *testing.T) {
			line := `{"level":"` + tc.level + `","message":"x","target":"t"}`
			got, ok := classifyLine(line)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantLevel, got.Level)
			}
		})
	}
}

// TestClassifyLineUnstructuredOutputIsAnError covers output that bypassed ADP's logger.
// Preflight mode forces JSON, so a non-JSON line means something wrote straight to the fd — a
// Rust panic, a dynamic linker failure, an allocator abort. All of those are serious.
func TestClassifyLineUnstructuredOutputIsAnError(t *testing.T) {
	for _, line := range []string{
		`thread 'main' panicked at bin/agent-data-plane/src/main.rs:42:5:`,
		`note: run with RUST_BACKTRACE=1 environment variable to display a backtrace`,
		`agent-data-plane: error while loading shared libraries: libssl.so.3: cannot open shared object file`,
		`memory allocation of 8589934592 bytes failed`,
	} {
		t.Run(line[:min(30, len(line))], func(t *testing.T) {
			got, ok := classifyLine(line)
			require.True(t, ok, "unstructured output must be reported")
			assert.Equal(t, levelError, got.Level)
			assert.Equal(t, targetUnstructured, got.Target)
		})
	}
}

// TestClassifyLineTruncatedRecordIsDropped guards against a false positive: a JSON record
// the capture buffer cut in half is not evidence that ADP misbehaved, and the truncation is
// already reported through findingOutputDropped.
func TestClassifyLineTruncatedRecordIsDropped(t *testing.T) {
	truncated := realErrorDsdBind[:200]
	_, ok := classifyLine(truncated)
	assert.False(t, ok, "a truncated JSON record must not be blamed on ADP")
}

func TestClassifyLineBlank(t *testing.T) {
	for _, line := range []string{"", "   ", "\t"} {
		_, ok := classifyLine(line)
		assert.False(t, ok)
	}
}

func TestClassifyLineEmptyMessage(t *testing.T) {
	got, ok := classifyLine(`{"level":"ERROR","message":"","target":"saluki_core"}`)
	require.True(t, ok)
	assert.Equal(t, "(no message)", got.Signature)
}

func TestClassifyLineMissingTarget(t *testing.T) {
	got, ok := classifyLine(`{"level":"ERROR","message":"boom"}`)
	require.True(t, ok)
	assert.Equal(t, "<unknown>", got.Target)
}

func TestNormalizeSignature(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "listener URIs are collapsed",
			message: "Failed to create unixgram listener on unixgram:///opt/datadog-agent/run/adp-preflight/dsd.socket",
			want:    "Failed to create unixgram listener on unixgram:<path>",
		},
		{
			name:    "windows paths are collapsed",
			message: `could not open C:\ProgramData\Datadog\run\adp-preflight\dsd.socket`,
			want:    `could not open <path>`,
		},
		{
			name:    "os error numbers are collapsed",
			message: "No such file or directory (os error 2)",
			want:    "No such file or directory (os error <n>)",
		},
		{
			name:    "uuids are collapsed",
			message: "session 3f2504e0-4f89-11d3-9a0c-0305e82c3301 failed",
			want:    "session <uuid> failed",
		},
		{
			name:    "hex addresses are collapsed",
			message: "segfault at 0xdeadBEEF12",
			want:    "segfault at <addr>",
		},
		{
			name:    "embedded newlines are folded",
			message: "outer\n\nCaused by:\n    inner",
			want:    "outer | | Caused by: | inner",
		},
		{
			name:    "whitespace is normalized",
			message: "  too    many   spaces  ",
			want:    "too many spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, signature(tt.message))
		})
	}
}

// TestNormalizeSignatureGroupsAcrossHosts is the property that actually matters: the same
// failure on two hosts, differing only in volatile detail, must produce one signature.
func TestNormalizeSignatureGroupsAcrossHosts(t *testing.T) {
	a := "Failed to read certificate file '/opt/datadog-agent/etc/ipc_cert.pem' after 20 seconds: No such file or directory (os error 2)"
	b := "Failed to read certificate file '/custom/root/etc/ipc_cert.pem' after 30 seconds: No such file or directory (os error 2)"
	assert.Equal(t, signature(a), signature(b))
}

func TestNormalizeSignatureIsBounded(t *testing.T) {
	assert.Len(t, signature(strings.Repeat("abcdefghij", 500)), maxSignatureLen)
}

func TestScanOutputRealCleanRun(t *testing.T) {
	// The full log of a healthy preflight mode run: only the standalone-mode warning is
	// notable, and there are no errors.
	scanned := scanOutput([]string{
		realInfoStartup,
		realWarnStandalone,
		realInfoListener,
		realInfoShutdown,
	})
	require.Len(t, scanned, 1)
	assert.Equal(t, levelWarn, scanned[0].Level)
	assert.False(t, hasErrors(scanned), "the standalone-mode warning must not count as an error")
	assert.False(t, hasUnexpectedWarnings(scanned),
		"preflight mode sets standalone_mode itself, so its warning must not be a finding on every run")
}

// TestScanOutputInvalidAPIKey is the case that motivated warnings_in_log. ADP reports a
// rejected API key at WARN, so before this the single most likely day-one problem produced
// no finding at all.
//
// Behaviour confirmed against agent-data-plane 1.4.0: a malformed key (abc123) and a
// well-formed but fake key (deadbeef...) both produce this record. A key that is one
// character repeated 32 times does not — ADP treats that as a placeholder and skips
// validation, which is worth knowing when choosing a test value.
func TestScanOutputInvalidAPIKey(t *testing.T) {
	scanned := scanOutput([]string{
		realInfoStartup,
		realWarnStandalone,
		realWarnInvalidAPIKey,
		realInfoListener,
	})

	assert.False(t, hasErrors(scanned), "ADP reports a bad API key at WARN, not ERROR")
	assert.True(t, hasUnexpectedWarnings(scanned), "a rejected API key must produce a finding")

	var found bool
	for _, l := range scanned {
		if strings.Contains(l.Signature, "Datadog API key is invalid") {
			found = true
			assert.Equal(t, "saluki_components::common::datadog::validation", l.Target)
		}
	}
	assert.True(t, found, "the invalid-key record must survive the scan")
}

func TestIsExpectedWarning(t *testing.T) {
	t.Run("the standalone-mode warning is expected", func(t *testing.T) {
		l, ok := classifyLine(realWarnStandalone)
		require.True(t, ok)
		assert.True(t, isExpectedWarning(l))
	})

	t.Run("an invalid API key is not expected", func(t *testing.T) {
		l, ok := classifyLine(realWarnInvalidAPIKey)
		require.True(t, ok)
		assert.False(t, isExpectedWarning(l))
	})

	t.Run("the same message from a different target is not expected", func(t *testing.T) {
		// Guards against the allowlist being matched on message alone, which would let an
		// unrelated component suppress a real warning.
		l, ok := classifyLine(`{"level":"WARN","message":"Running in standalone mode.","target":"saluki_core::topology"}`)
		require.True(t, ok)
		assert.False(t, isExpectedWarning(l))
	})
}

func TestScanOutputRealFailedRun(t *testing.T) {
	scanned := scanOutput([]string{
		realInfoStartup,
		realWarnStandalone,
		realErrorSupervisor,
		realErrorDsdBind,
		realInfoShutdown,
	})
	assert.True(t, hasErrors(scanned))
	// One warning plus two distinct errors: they share a root cause but come from
	// different targets with different messages, so both are worth reporting.
	require.Len(t, scanned, 3)
}

func TestScanOutputDeduplicates(t *testing.T) {
	// The same failure logged twice, differing only in timestamp and a retry count.
	first := `{"level":"ERROR","message":"connection refused (attempt 1)","target":"saluki_io::net"}`
	second := `{"level":"ERROR","message":"connection refused (attempt 2)","target":"saluki_io::net"}`
	// Same message, different target: must NOT be deduplicated.
	other := `{"level":"ERROR","message":"connection refused (attempt 3)","target":"saluki_core::topology"}`

	scanned := scanOutput([]string{first, second, other})
	require.Len(t, scanned, 2)
	assert.Equal(t, "saluki_io::net", scanned[0].Target)
	assert.Equal(t, "saluki_core::topology", scanned[1].Target)
}

func TestOutcomeResult(t *testing.T) {
	t.Run("no findings is clean", func(t *testing.T) {
		assert.Equal(t, resultClean, (&outcome{}).result())
	})

	t.Run("the first finding wins", func(t *testing.T) {
		o := &outcome{}
		o.add(findingProbeFailed)
		o.add(findingErrorsInLog)
		assert.Equal(t, string(findingProbeFailed), o.result())
	})

	t.Run("findings are deduplicated", func(t *testing.T) {
		o := &outcome{}
		o.add(findingErrorsInLog)
		o.add(findingErrorsInLog)
		assert.Equal(t, []finding{findingErrorsInLog}, o.findings)
	})
}
