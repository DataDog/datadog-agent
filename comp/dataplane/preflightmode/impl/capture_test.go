// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
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

// captureOf feeds whole lines through a capture and returns what it retained. The tests go
// through Write rather than calling parseRecord directly wherever the property under test is
// about the retained set, because ingest is where deduplication and the caps live.
func captureOf(t *testing.T, lines ...string) ([]logRecord, int) {
	t.Helper()
	c := newCapture(logmock.New(t))
	for _, line := range lines {
		_, err := fmt.Fprintln(c, line)
		require.NoError(t, err)
	}
	return c.finish()
}

// notableRecords returns the records preflight mode reports on, as report does.
func notableRecords(records []logRecord) []logRecord {
	var out []logRecord
	for _, rec := range records {
		if rec.notable() {
			out = append(out, rec)
		}
	}
	return out
}

// signatures returns the retained signatures, for tests about line splitting rather than
// parsing.
func signatures(records []logRecord) []string {
	out := make([]string, 0, len(records))
	for _, rec := range records {
		out = append(out, rec.Signature)
	}
	return out
}

// jsonRecord builds an ADP-shaped log record of roughly the requested length.
func jsonRecord(t *testing.T, level, target string, padTo int) string {
	t.Helper()
	rec, err := json.Marshal(map[string]any{
		"level":   level,
		"target":  target,
		"message": strings.Repeat("x", padTo),
	})
	require.NoError(t, err)
	return string(rec)
}

func TestCaptureSplitsLines(t *testing.T) {
	// Non-JSON lines, so each one is retained as unstructured output with the line itself as
	// its signature — which is what makes the split observable.
	records, dropped := captureOf(t, "one", "two", "three")
	assert.Equal(t, []string{"one", "two", "three"}, signatures(records))
	assert.Zero(t, dropped)
}

func TestCaptureHandlesCRLFAndPartialWrites(t *testing.T) {
	c := newCapture(logmock.New(t))
	// A line delivered across several writes, with Windows line endings.
	fmt.Fprint(c, "hel")
	fmt.Fprint(c, "lo\r\nwor")
	fmt.Fprint(c, "ld\r\n")

	records, dropped := c.snapshot()
	assert.Equal(t, []string{"hello", "world"}, signatures(records))
	assert.Zero(t, dropped)
}

func TestCaptureFlushesUnterminatedTrailingLine(t *testing.T) {
	c := newCapture(logmock.New(t))
	fmt.Fprint(c, "terminated\nunterminated")

	records, _ := c.finish()
	assert.Equal(t, []string{"terminated", "unterminated"}, signatures(records))
}

// TestCaptureOversizedRecordIsDroppedWhole is a regression test.
//
// An over-long record used to be truncated silently, without counting it. A truncated record is
// no longer valid JSON, so the scan discarded it (correctly — see parseRecord), and with
// nothing counted the run reported "clean", losing the error entirely. Records are now kept
// whole or dropped whole, and a drop is always counted so run() reports findingOutputDropped.
func TestCaptureOversizedRecordIsDroppedWhole(t *testing.T) {
	oversized := jsonRecord(t, "ERROR", "agent_data_plane", maxLineBytes+1000)

	records, dropped := captureOf(t, oversized)
	assert.Empty(t, records, "an over-long record is dropped, not truncated into garbage")
	assert.Positive(t, dropped, "dropping a record loses information and must be reported")
}

// TestCaptureChunkedLongLineDoesNotFabricateFindings is a regression test.
//
// os/exec copies the child's output in 32 KiB chunks, so one long record arrives across
// several Write calls. The overflow path used to emit a fragment and then discard the rest
// of the pending buffer, so subsequent chunks began mid-record. Those fragments do not start
// with '{', so parseRecord treated them as output that bypassed ADP's logger and manufactured
// an error — from a single oversized INFO record.
func TestCaptureChunkedLongLineDoesNotFabricateFindings(t *testing.T) {
	c := newCapture(logmock.New(t))
	huge := jsonRecord(t, "INFO", "agent_data_plane::cli::run", maxLineBytes*2)

	// Deliver it the way os/exec does.
	const chunk = 32 * 1024
	for i := 0; i < len(huge); i += chunk {
		_, err := fmt.Fprint(c, huge[i:min(i+chunk, len(huge))])
		require.NoError(t, err)
	}
	fmt.Fprint(c, "\n")

	records, dropped := c.finish()
	assert.Positive(t, dropped, "the record was too long to keep, which must be reported")
	assert.False(t, hasErrors(records),
		"a dropped INFO record must not manufacture an error; got %+v", records)
}

// TestCaptureChunkedNormalLinesAreReassembled is the non-pathological version: ordinary
// records split across writes must come back intact, or every long-ish ADP error would be
// mangled into unparseable fragments.
func TestCaptureChunkedNormalLinesAreReassembled(t *testing.T) {
	c := newCapture(logmock.New(t))
	rec := jsonRecord(t, "ERROR", "saluki_core::runtime::supervisor", 100)

	for _, part := range []string{rec[:20], rec[20:60], rec[60:], "\n"} {
		fmt.Fprint(c, part)
	}

	records, dropped := c.snapshot()
	assert.Zero(t, dropped)
	require.Len(t, records, 1)
	assert.Equal(t, levelError, records[0].Level)
	assert.Equal(t, "saluki_core::runtime::supervisor", records[0].Target)
}

// TestCaptureDeduplicatesRecords is what bounds the common pathological case: a process
// looping on one failure costs a single record however many times it logs it.
func TestCaptureDeduplicatesRecords(t *testing.T) {
	// The same failure logged twice, differing only in a retry count.
	first := `{"level":"ERROR","message":"connection refused (attempt 1)","target":"saluki_io::net"}`
	second := `{"level":"ERROR","message":"connection refused (attempt 2)","target":"saluki_io::net"}`
	// Same message, different target: must NOT be deduplicated.
	other := `{"level":"ERROR","message":"connection refused (attempt 3)","target":"saluki_core::topology"}`

	records, dropped := captureOf(t, first, second, other)
	require.Len(t, records, 2)
	assert.Equal(t, "saluki_io::net", records[0].Target)
	assert.Equal(t, "saluki_core::topology", records[1].Target)
	assert.Zero(t, dropped, "a duplicate loses no information, so it is not a drop")
}

func TestCaptureRecordCap(t *testing.T) {
	// Distinct targets, because distinct messages would collapse: the digits are scrubbed.
	lines := make([]string, 0, maxRecords+50)
	for i := 0; i < maxRecords+50; i++ {
		lines = append(lines, fmt.Sprintf(`{"level":"ERROR","message":"boom","target":"t%d"}`, i))
	}

	records, dropped := captureOf(t, lines...)
	assert.Len(t, records, maxRecords)
	assert.Equal(t, 50, dropped)
}

// TestCaptureContextRecordsCannotCrowdOutErrors is the reason the context share is capped
// separately. Records below a warning are retained as the trail leading up to a failure, but a
// chatty stream of them must not consume the buffer before the failure arrives — that would
// turn a real error into nothing but findingOutputDropped.
func TestCaptureContextRecordsCannotCrowdOutErrors(t *testing.T) {
	lines := make([]string, 0, maxContextRecords+51)
	for i := 0; i < maxContextRecords+50; i++ {
		lines = append(lines, fmt.Sprintf(`{"level":"INFO","message":"chatter","target":"t%d"}`, i))
	}
	lines = append(lines, realErrorCertMissing)

	records, dropped := captureOf(t, lines...)
	assert.Positive(t, dropped, "the surplus context records must be reported as dropped")
	assert.True(t, hasErrors(records), "the error must survive a flood of context records")
	assert.Len(t, notableRecords(records), 1)
}

// TestCaptureDeduplicatesBeyondTheBounds pins an emergent property of bounding the fields
// before deduplicating: two records that differ only past the bounds collapse into one. That is
// the behaviour we want — ADP renders whole error chains into a message, and two chains that
// agree for their first maxSignatureLen bytes are the same failure for grouping purposes — but
// it is easy to break by reordering parseRecord, so it is pinned here.
func TestCaptureDeduplicatesBeyondTheBounds(t *testing.T) {
	prefix := strings.Repeat("x", maxSignatureLen)
	records, dropped := captureOf(t,
		`{"level":"ERROR","target":"saluki_io::net","message":"`+prefix+`differs here"}`,
		`{"level":"ERROR","target":"saluki_io::net","message":"`+prefix+`and here"}`,
	)
	assert.Len(t, records, 1)
	assert.Zero(t, dropped)
}

// TestCaptureRetentionIsBounded pins the memory bound, which is the whole point of the caps:
// every retained field is bounded, so the worst case is arithmetic and not a running total.
func TestCaptureRetentionIsBounded(t *testing.T) {
	lines := make([]string, 0, maxRecords*2)
	for i := 0; i < maxRecords*2; i++ {
		// Both fields well past their bounds. The index leads the target so the records stay
		// distinct once it is truncated — see TestCaptureDeduplicatesBeyondTheBounds.
		lines = append(lines, jsonRecord(t, "ERROR", fmt.Sprintf("t%d%s", i, strings.Repeat("x", maxTargetLen*4)), maxSignatureLen*4))
	}

	records, dropped := captureOf(t, lines...)
	assert.Positive(t, dropped)

	total := 0
	for _, rec := range records {
		assert.LessOrEqual(t, len(rec.Signature), maxSignatureLen)
		assert.LessOrEqual(t, len(rec.Target), maxTargetLen)
		total += len(rec.Signature) + len(rec.Target)
	}
	assert.LessOrEqual(t, total, maxRecords*(maxSignatureLen+maxTargetLen))
}

func TestCaptureBlankLinesAreIgnored(t *testing.T) {
	c := newCapture(logmock.New(t))
	fmt.Fprint(c, "\n\n\nreal\n\n")

	records, dropped := c.snapshot()
	assert.Equal(t, []string{"real"}, signatures(records))
	assert.Zero(t, dropped)
}

// TestCaptureSnapshotIsRepeatable guards against snapshot() being destructive. The lifecycle
// tests poll it while ADP is still running, and a snapshot that parsed a partial line would
// split one record into two — the second of which has no '{' prefix and would be reported as
// unstructured output.
func TestCaptureSnapshotIsRepeatable(t *testing.T) {
	c := newCapture(logmock.New(t))
	rec := jsonRecord(t, "ERROR", "saluki_io::net", 50)

	// Mid-record snapshot, exactly what the lifecycle tests do.
	fmt.Fprint(c, rec[:30])
	mid, _ := c.snapshot()
	assert.Empty(t, mid, "an incomplete record must not be parsed")

	fmt.Fprint(c, rec[30:]+"\n")
	final, dropped := c.finish()
	require.Len(t, final, 1, "the record must survive an interleaved snapshot")
	assert.Equal(t, levelError, final[0].Level)
	assert.Zero(t, dropped)

	again, _ := c.snapshot()
	assert.Equal(t, final, again, "snapshot must be repeatable")
}

func TestCaptureWriteReportsFullLength(t *testing.T) {
	// io.Writer must report n == len(p) or os/exec's io.Copy treats it as a short write.
	c := newCapture(logmock.New(t))
	p := []byte(strings.Repeat("z", 8192) + "\n")
	n, err := c.Write(p)
	require.NoError(t, err)
	assert.Equal(t, len(p), n)
}

func TestParseRecordRealRecords(t *testing.T) {
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
			name:            "INFO is kept as context",
			line:            realInfoStartup,
			wantOK:          true,
			wantLevel:       "INFO",
			wantTarget:      "agent_data_plane::cli::run",
			wantSigContains: "Agent Data Plane starting",
		},
		{
			name:            "INFO with nested spans",
			line:            realInfoListener,
			wantOK:          true,
			wantLevel:       "INFO",
			wantTarget:      "saluki_components::sources::dogstatsd",
			wantSigContains: "DogStatsD listener started",
		},
		{
			name:            "INFO shutdown",
			line:            realInfoShutdown,
			wantOK:          true,
			wantLevel:       "INFO",
			wantTarget:      "agent_data_plane::cli::run",
			wantSigContains: "shut down successfully",
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
			got, ok := parseRecord(tt.line)
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

// TestParseRecordMultiLineMessageStaysOneRecord is the property that justifies forcing
// JSON: ADP renders a whole error chain into a single message field, and with JSON there is
// never any ambiguity about where one log event ends.
func TestParseRecordMultiLineMessageStaysOneRecord(t *testing.T) {
	got, ok := parseRecord(realErrorDsdBind)
	require.True(t, ok)
	assert.Equal(t, levelError, got.Level)
	assert.NotContains(t, got.Signature, "\n", "the signature must stay on one line")
	// The chain is preserved, because it is the diagnostic part.
	assert.Contains(t, got.Signature, "Failed to create unixgram listener")
	assert.Contains(t, got.Signature, "Invalid argument (os error <n>)")
}

func TestParseRecordLevels(t *testing.T) {
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
		{"INFO", true, "INFO"},
		{"info", true, "INFO"},
		{"DEBUG", true, "DEBUG"},
		{"TRACE", true, "TRACE"},
		// Not a level the tracing crate emits, so the line did not come from ADP's logger.
		{"NOTICE", false, ""},
		{"", false, ""},
	} {
		t.Run("level "+tc.level, func(t *testing.T) {
			line := `{"level":"` + tc.level + `","message":"x","target":"t"}`
			got, ok := parseRecord(line)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantLevel, got.Level)
			}
		})
	}
}

// TestParseRecordUnstructuredOutputIsAnError covers output that bypassed ADP's logger.
// Preflight mode forces JSON, so a non-JSON line means something wrote straight to the fd — a
// Rust panic, a dynamic linker failure, an allocator abort. All of those are serious.
func TestParseRecordUnstructuredOutputIsAnError(t *testing.T) {
	for _, line := range []string{
		`thread 'main' panicked at bin/agent-data-plane/src/main.rs:42:5:`,
		`note: run with RUST_BACKTRACE=1 environment variable to display a backtrace`,
		`agent-data-plane: error while loading shared libraries: libssl.so.3: cannot open shared object file`,
		`memory allocation of 8589934592 bytes failed`,
	} {
		t.Run(line[:min(30, len(line))], func(t *testing.T) {
			got, ok := parseRecord(line)
			require.True(t, ok, "unstructured output must be reported")
			assert.Equal(t, levelError, got.Level)
			assert.Equal(t, targetUnstructured, got.Target)
		})
	}
}

// TestParseRecordPartialRecordIsDropped guards against a false positive: half a JSON record is
// not evidence that ADP misbehaved, and the loss is already reported through
// findingOutputDropped.
func TestParseRecordPartialRecordIsDropped(t *testing.T) {
	_, ok := parseRecord(realErrorDsdBind[:200])
	assert.False(t, ok, "a partial JSON record must not be blamed on ADP")
}

func TestParseRecordBlank(t *testing.T) {
	for _, line := range []string{"", "   ", "\t"} {
		_, ok := parseRecord(line)
		assert.False(t, ok)
	}
}

func TestParseRecordEmptyMessage(t *testing.T) {
	got, ok := parseRecord(`{"level":"ERROR","message":"","target":"saluki_core"}`)
	require.True(t, ok)
	assert.Equal(t, "(no message)", got.Signature)
}

func TestParseRecordMissingTarget(t *testing.T) {
	got, ok := parseRecord(`{"level":"ERROR","message":"boom"}`)
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

func TestCaptureRealCleanRun(t *testing.T) {
	// The full log of a healthy preflight mode run: only the standalone-mode warning is
	// notable, and there are no errors.
	records, dropped := captureOf(t, realInfoStartup, realWarnStandalone, realInfoListener, realInfoShutdown)
	assert.Zero(t, dropped)
	assert.False(t, hasErrors(records), "the standalone-mode warning must not count as an error")
	assert.False(t, hasUnexpectedWarnings(records),
		"preflight mode sets standalone_mode itself, so its warning must not be a finding on every run")

	require.Len(t, notableRecords(records), 1)
	assert.Equal(t, levelWarn, notableRecords(records)[0].Level)
	// The INFO records are kept too, so a reader has the whole run and not just its problems.
	assert.Len(t, records, 4)
}

// TestCaptureInvalidAPIKey is the case that motivated warnings_in_log. ADP reports a
// rejected API key at WARN, so before this the single most likely day-one problem produced
// no finding at all.
//
// Behaviour confirmed against agent-data-plane 1.4.0: a malformed key (abc123) and a
// well-formed but fake key (deadbeef...) both produce this record. A key that is one
// character repeated 32 times does not — ADP treats that as a placeholder and skips
// validation, which is worth knowing when choosing a test value.
func TestCaptureInvalidAPIKey(t *testing.T) {
	records, _ := captureOf(t, realInfoStartup, realWarnStandalone, realWarnInvalidAPIKey, realInfoListener)

	assert.False(t, hasErrors(records), "ADP reports a bad API key at WARN, not ERROR")
	assert.True(t, hasUnexpectedWarnings(records), "a rejected API key must produce a finding")

	var found bool
	for _, rec := range records {
		if strings.Contains(rec.Signature, "Datadog API key is invalid") {
			found = true
			assert.Equal(t, "saluki_components::common::datadog::validation", rec.Target)
		}
	}
	assert.True(t, found, "the invalid-key record must be retained")
}

func TestCaptureRealFailedRun(t *testing.T) {
	records, _ := captureOf(t, realInfoStartup, realWarnStandalone, realErrorSupervisor, realErrorDsdBind, realInfoShutdown)
	assert.True(t, hasErrors(records))
	// One warning plus two distinct errors: they share a root cause but come from
	// different targets with different messages, so both are worth reporting.
	require.Len(t, notableRecords(records), 3)
}

func TestIsExpectedWarning(t *testing.T) {
	t.Run("the standalone-mode warning is expected", func(t *testing.T) {
		rec, ok := parseRecord(realWarnStandalone)
		require.True(t, ok)
		assert.True(t, isExpectedWarning(rec))
	})

	t.Run("an invalid API key is not expected", func(t *testing.T) {
		rec, ok := parseRecord(realWarnInvalidAPIKey)
		require.True(t, ok)
		assert.False(t, isExpectedWarning(rec))
	})

	t.Run("the same message from a different target is not expected", func(t *testing.T) {
		// Guards against the allowlist being matched on message alone, which would let an
		// unrelated component suppress a real warning.
		rec, ok := parseRecord(`{"level":"WARN","message":"Running in standalone mode.","target":"saluki_core::topology"}`)
		require.True(t, ok)
		assert.False(t, isExpectedWarning(rec))
	})
}
