// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var testSum = [32]byte{0xde, 0xad, 0xbe, 0xef}

const testSumHex = "deadbeef00000000000000000000000000000000000000000000000000000000"

func testExecFile() ExecFile {
	return ExecFile{
		PID:         42,
		ContainerID: containerutils.ContainerID("abc123"),
		Path:        "/usr/bin/evil tool",
	}
}

// captureLogs redirects the global logger to a buffer at level for the duration of the test,
// and returns a function flushing and returning the lines logged so far
func captureLogs(t *testing.T, level string) func() []string {
	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	w := bufio.NewWriter(&buf)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.TraceLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, level)
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })

	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		log.Flush()
		require.NoError(t, w.Flush())
		out := strings.TrimSpace(buf.String())
		if out == "" {
			return nil
		}
		return strings.Split(out, "\n")
	}
}

func TestFormatMatch(t *testing.T) {
	matches := []Match{
		{Rule: "Evil_A", Namespace: "malware", Tags: []string{"trojan", "linux"}},
		{Rule: "Evil_B", Namespace: "malware", Tags: []string{"linux"}},
		{Rule: "Miner", Namespace: "crypto"},
	}
	f := testExecFile()
	f.IsScript = true

	assert.Equal(t,
		`yara: match path="/usr/bin/evil tool" pid=42 container_id=abc123 script=true sha256=`+testSumHex+
			` rules_version=v1 rules=Evil_A,Evil_B,Miner namespaces=crypto,malware tags=linux,trojan`,
		formatMatch(f, testSum, "v1", matches))
}

func TestFormatQuotesPath(t *testing.T) {
	f := testExecFile()
	f.Path = "/tmp/x\nyara: match path=/fake"
	line := formatNoMatch(f, testSum, "v1")
	assert.NotContains(t, line, "\n")
	assert.Contains(t, line, `path="/tmp/x\nyara: match path=/fake"`)
}

func TestFormatHostExec(t *testing.T) {
	f := testExecFile()
	f.ContainerID = ""
	assert.Equal(t,
		`yara: no match path="/usr/bin/evil tool" pid=42 container_id= script=false sha256=`+testSumHex+` rules_version=v1`,
		formatNoMatch(f, testSum, "v1"))
}

func TestFormatError(t *testing.T) {
	err := errors.New("context deadline exceeded")
	assert.Equal(t,
		`yara: scan failed path="/usr/bin/evil tool" pid=42 container_id=abc123 script=false sha256=`+testSumHex+
			` rules_version=v1 error="context deadline exceeded"`,
		formatError(testExecFile(), testSum, "v1", err, 0))
	assert.True(t, strings.HasSuffix(formatError(testExecFile(), testSum, "v1", err, 7), " suppressed_errors=7"))
}

func TestStructuredReporterMatch(t *testing.T) {
	logs := captureLogs(t, "info")
	r := NewStructuredReporter("v1", StructuredReporterOptions{})

	r.Report(testExecFile(), testSum, []Match{{Rule: "A", Namespace: "ns"}, {Rule: "B", Namespace: "ns"}}, nil)

	lines := logs()
	require.Len(t, lines, 1, "one line per report, not per match")
	assert.True(t, strings.HasPrefix(lines[0], "[INFO] yara: match "), lines[0])
	assert.Contains(t, lines[0], "rules=A,B namespaces=ns")
}

func TestStructuredReporterNoMatch(t *testing.T) {
	logs := captureLogs(t, "info")
	r := NewStructuredReporter("v1", StructuredReporterOptions{})
	r.Report(testExecFile(), testSum, nil, nil)
	assert.Empty(t, logs(), "no match is not logged above debug")

	logs = captureLogs(t, "debug")
	r.Report(testExecFile(), testSum, nil, nil)
	lines := logs()
	require.Len(t, lines, 1)
	assert.True(t, strings.HasPrefix(lines[0], "[DEBUG] yara: no match "), lines[0])
}

func TestStructuredReporterErrorRateLimit(t *testing.T) {
	logs := captureLogs(t, "info")
	r := NewStructuredReporter("v1", StructuredReporterOptions{ErrorLogBurst: 3, ErrorLogInterval: time.Hour})
	scanErr := errors.New("boom")

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Report(testExecFile(), testSum, nil, scanErr)
		}()
	}
	wg.Wait()

	lines := logs()
	require.Len(t, lines, 3, "only the burst gets through")
	for _, line := range lines {
		assert.True(t, strings.HasPrefix(line, "[WARN] yara: scan failed "), line)
	}
	assert.Equal(t, int64(97), r.suppressed.Load())

	// matches are never rate limited
	r.Report(testExecFile(), testSum, []Match{{Rule: "A"}}, nil)
	assert.Len(t, logs(), 4)
}

func TestStructuredReporterReportsSuppressedCount(t *testing.T) {
	logs := captureLogs(t, "info")
	r := NewStructuredReporter("v1", StructuredReporterOptions{ErrorLogBurst: 1, ErrorLogInterval: 50 * time.Millisecond})
	scanErr := errors.New("boom")

	r.Report(testExecFile(), testSum, nil, scanErr) // logged
	r.Report(testExecFile(), testSum, nil, scanErr) // suppressed
	r.Report(testExecFile(), testSum, nil, scanErr) // suppressed
	time.Sleep(60 * time.Millisecond)
	r.Report(testExecFile(), testSum, nil, scanErr) // logged, with the suppressed count

	lines := logs()
	require.Len(t, lines, 2)
	assert.NotContains(t, lines[0], "suppressed_errors")
	assert.True(t, strings.HasSuffix(lines[1], " suppressed_errors=2"), lines[1])
}
