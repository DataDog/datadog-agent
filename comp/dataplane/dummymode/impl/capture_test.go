// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package dummymodeimpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

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
	c := newCapture(logmock.New(t))
	_, err := fmt.Fprint(c, "one\ntwo\nthree\n")
	require.NoError(t, err)

	lines, dropped := c.snapshot()
	assert.Equal(t, []string{"one", "two", "three"}, lines)
	assert.Zero(t, dropped)
}

func TestCaptureHandlesCRLFAndPartialWrites(t *testing.T) {
	c := newCapture(logmock.New(t))
	// A line delivered across several writes, with Windows line endings.
	fmt.Fprint(c, "hel")
	fmt.Fprint(c, "lo\r\nwor")
	fmt.Fprint(c, "ld\r\n")

	lines, dropped := c.snapshot()
	assert.Equal(t, []string{"hello", "world"}, lines)
	assert.Zero(t, dropped)
}

func TestCaptureFlushesUnterminatedTrailingLine(t *testing.T) {
	c := newCapture(logmock.New(t))
	fmt.Fprint(c, "terminated\nunterminated")

	lines, _ := c.finish()
	assert.Equal(t, []string{"terminated", "unterminated"}, lines)
}

// TestCaptureOversizedRecordIsDroppedWhole is a regression test.
//
// An over-long record used to be truncated silently, without counting it. A truncated record is
// no longer valid JSON, so the scan discarded it (correctly — see classifyLine), and with
// nothing counted the run reported "clean", losing the error entirely. Records are now kept
// whole or dropped whole, and a drop is always counted so run() reports findingOutputDropped.
func TestCaptureOversizedRecordIsDroppedWhole(t *testing.T) {
	c := newCapture(logmock.New(t))
	oversized := jsonRecord(t, "ERROR", "agent_data_plane", maxCaptureBytes+1000)

	fmt.Fprintln(c, oversized)

	lines, dropped := c.finish()
	assert.Empty(t, lines, "an over-long record is dropped, not truncated into garbage")
	assert.Positive(t, dropped, "dropping a record loses information and must be reported")
}

// TestCaptureChunkedLongLineDoesNotFabricateFindings is a regression test.
//
// os/exec copies the child's output in 32 KiB chunks, so one long record arrives across
// several Write calls. The overflow path used to emit a fragment and then discard the rest
// of the pending buffer, so subsequent chunks began mid-record. Those fragments do not start
// with '{', so classifyLine treated them as output that bypassed ADP's logger and manufactured
// an error — from a single oversized INFO record.
func TestCaptureChunkedLongLineDoesNotFabricateFindings(t *testing.T) {
	c := newCapture(logmock.New(t))
	huge := jsonRecord(t, "INFO", "agent_data_plane::cli::run", maxCaptureBytes*2)

	// Deliver it the way os/exec does.
	const chunk = 32 * 1024
	for i := 0; i < len(huge); i += chunk {
		_, err := fmt.Fprint(c, huge[i:min(i+chunk, len(huge))])
		require.NoError(t, err)
	}
	fmt.Fprint(c, "\n")

	lines, dropped := c.finish()
	assert.Positive(t, dropped, "the record was too long to keep, which must be reported")

	scanned := scanOutput(lines)
	assert.False(t, hasErrors(scanned),
		"a dropped INFO record must not manufacture an error; got %+v", scanned)
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

	lines, dropped := c.snapshot()
	require.Equal(t, []string{rec}, lines)
	assert.Zero(t, dropped)

	scanned := scanOutput(lines)
	require.Len(t, scanned, 1)
	assert.Equal(t, levelError, scanned[0].Level)
}

func TestCaptureLineCountCap(t *testing.T) {
	c := newCapture(logmock.New(t))
	for i := 0; i < maxCaptureLines+50; i++ {
		fmt.Fprintf(c, "line %d\n", i)
	}

	lines, dropped := c.snapshot()
	assert.Len(t, lines, maxCaptureLines)
	assert.Equal(t, 50, dropped)
}

func TestCaptureByteCap(t *testing.T) {
	c := newCapture(logmock.New(t))
	// Each line is small but together they exceed maxCaptureBytes.
	line := strings.Repeat("y", 1024)
	for i := 0; i < (maxCaptureBytes/1024)+20; i++ {
		fmt.Fprintln(c, line)
	}

	lines, dropped := c.snapshot()
	assert.Positive(t, dropped)
	total := 0
	for _, l := range lines {
		total += len(l)
	}
	assert.LessOrEqual(t, total, maxCaptureBytes)
}

func TestCaptureBlankLinesAreIgnored(t *testing.T) {
	c := newCapture(logmock.New(t))
	fmt.Fprint(c, "\n\n\nreal\n\n")

	lines, dropped := c.snapshot()
	assert.Equal(t, []string{"real"}, lines)
	assert.Zero(t, dropped)
}

// TestCaptureSnapshotIsRepeatable guards against snapshot() being destructive. The lifecycle
// tests poll it while ADP is still running, and a snapshot that committed a partial line
// would split one record into two — the second of which has no '{' prefix and would be
// reported as unstructured output.
func TestCaptureSnapshotIsRepeatable(t *testing.T) {
	c := newCapture(logmock.New(t))
	rec := jsonRecord(t, "ERROR", "saluki_io::net", 50)

	// Mid-record snapshot, exactly what the lifecycle tests do.
	fmt.Fprint(c, rec[:30])
	mid, _ := c.snapshot()
	assert.Empty(t, mid, "an incomplete record must not be committed")

	fmt.Fprint(c, rec[30:]+"\n")
	final, dropped := c.finish()
	require.Equal(t, []string{rec}, final, "the record must survive an interleaved snapshot")
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
