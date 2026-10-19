// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestCappedBufferUnderLimit(t *testing.T) {
	c := &cappedBuffer{limit: 100}
	n, err := c.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, "abc", string(c.Bytes()))
	assert.False(t, c.truncated)
}

func TestCappedBufferTruncatesAcrossWrites(t *testing.T) {
	c := &cappedBuffer{limit: 5}

	n, _ := c.Write([]byte("abc"))
	assert.Equal(t, 3, n)
	assert.False(t, c.truncated)

	// This write crosses the limit: only 2 bytes fit, but Write must still report
	// the full length so the child process never sees a short/broken write.
	n, _ = c.Write([]byte("defgh"))
	assert.Equal(t, 5, n)
	assert.True(t, c.truncated)
	assert.Equal(t, "abcde", string(c.Bytes()))

	// Once full, further writes are discarded and stay flagged as truncated.
	n, _ = c.Write([]byte("xyz"))
	assert.Equal(t, 3, n)
	assert.True(t, c.truncated)
	assert.Equal(t, "abcde", string(c.Bytes()))
}

func TestCappedBufferExactFill(t *testing.T) {
	c := &cappedBuffer{limit: 5}
	n, _ := c.Write([]byte("abcde"))
	assert.Equal(t, 5, n)
	assert.False(t, c.truncated, "filling to exactly the limit is not truncation")
	assert.Equal(t, "abcde", string(c.Bytes()))
}

func TestParseRows(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantLen int
		wantErr bool
	}{
		{"empty", "", 0, false},
		{"whitespace", "  \n ", 0, false},
		{"null", "null", 0, false},
		{"array", `[{"Name":"a"},{"Name":"b"}]`, 2, false},
		{"single object coerced to one row", `{"Name":"a"}`, 1, false},
		{"truncated/malformed", `[{"Name":"a`, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := parseRows([]byte(tc.in))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rows, tc.wantLen)
		})
	}
}

func TestRestrictedEnvFiltersToAllowlist(t *testing.T) {
	// TMP is in requiredEnvVars; the DD_ var is not and must be dropped.
	t.Setenv("TMP", `C:\tmp-test`)
	t.Setenv("DD_POWERSHELL_TEST_SECRET", "leak")

	env := restrictedEnv()

	var sawTMP, sawSecret bool
	for _, e := range env {
		upper := strings.ToUpper(e)
		if strings.HasPrefix(upper, "TMP=") {
			sawTMP = true
		}
		if strings.HasPrefix(upper, "DD_POWERSHELL_TEST_SECRET=") {
			sawSecret = true
		}
	}
	assert.True(t, sawTMP, "allowlisted TMP should be passed through")
	assert.False(t, sawSecret, "non-allowlisted vars must be dropped")
}

func newTestCheck() *PowershellCheck {
	return &PowershellCheck{instance: &instanceConfig{Cmdlet: "Get-Service"}}
}

func TestSubmitMetricVirtual(t *testing.T) {
	c := newTestCheck()
	m := mocksender.NewMockSender(t, "powershell-test")
	tags := []string{"service:dnscache"}
	m.On("Gauge", "service.up", float64(1), "", tags).Return()

	err := c.submitMetric(m, &metricEntry{Property: "1", Name: "service.up", Type: "gauge"},
		map[string]interface{}{}, tags)

	require.NoError(t, err)
	m.AssertExpectations(t)
}

func TestSubmitMetricTypeDispatch(t *testing.T) {
	cases := []struct {
		metricType string
		method     string
	}{
		{"gauge", "Gauge"},
		{"", "Gauge"},
		{"rate", "Rate"},
		{"count", "Count"},
		{"monotonic_count", "MonotonicCount"},
		{"histogram", "Histogram"},
		{"distribution", "Distribution"},
	}
	for _, tc := range cases {
		t.Run(tc.metricType, func(t *testing.T) {
			c := newTestCheck()
			m := mocksender.NewMockSender(t, "powershell-test")
			tags := []string{"k:v"}
			m.On(tc.method, "svc.value", float64(7), "", tags).Return()

			err := c.submitMetric(m, &metricEntry{Property: "Value", Name: "svc.value", Type: tc.metricType},
				map[string]interface{}{"Value": float64(7)}, tags)

			require.NoError(t, err)
			m.AssertExpectations(t)
		})
	}
}

func TestSubmitMetricErrors(t *testing.T) {
	t.Run("missing property fails the run", func(t *testing.T) {
		c := newTestCheck()
		m := mocksender.NewMockSender(t, "powershell-test")
		err := c.submitMetric(m, &metricEntry{Property: "Absent", Name: "svc.x", Type: "gauge"},
			map[string]interface{}{"Other": float64(1)}, nil)
		require.Error(t, err)
		m.AssertNotCalled(t, "Gauge")
	})

	t.Run("non-numeric value fails the run", func(t *testing.T) {
		c := newTestCheck()
		m := mocksender.NewMockSender(t, "powershell-test")
		err := c.submitMetric(m, &metricEntry{Property: "Status", Name: "svc.status", Type: "gauge"},
			map[string]interface{}{"Status": "running"}, nil)
		require.Error(t, err)
		m.AssertNotCalled(t, "Gauge")
	})
}

// newExecTestCheck builds a check wired to spawn a real powershell.exe, with the
// cmdlet allowlisted and its module unpinned.
func newExecTestCheck(cmdlet string) *PowershellCheck {
	return &PowershellCheck{
		instance: &instanceConfig{Timeout: defaultTimeout, MaxOutputBytes: defaultMaxOutputBytes},
		allowlist: &allowlist{
			Version:        allowlistVersion,
			AllowedCmdlets: map[string]allowedCmdlet{cmdlet: {Module: "*"}},
		},
	}
}

func requirePowerShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("powershell.exe is not available")
	}
}

// The three tests below spawn a real powershell.exe. They are the only coverage of
// the stdin + explicit-UTF-8 + ConvertFrom-Json + -Command contract, which no
// Go-only test can reach: a stub would assert our assumptions against themselves.

// A non-ASCII value must survive the round-trip byte for byte. Decoding either
// direction with the OEM codepage instead of UTF-8 makes this filter match nothing.
func TestRunCmdletRoundTripsNonASCIIValues(t *testing.T) {
	requirePowerShell(t)

	dir := t.TempDir()
	// A non-ASCII letter plus U+2019, which PowerShell treats as a quote character.
	name := "café-’quoted’.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
	// A decoy, so a mangled or over-broad match cannot pass by accident.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "decoy.txt"), []byte("x"), 0o600))

	c := newExecTestCheck("Get-ChildItem")
	rows, err := c.runCmdlet("Get-ChildItem",
		[]parameterEntry{{Name: "Path", Value: dir}},
		[]whereEntry{{Property: "Name", Op: "eq", Value: name}},
		[]string{"Name"})
	require.NoError(t, err)
	require.Len(t, rows, 1, "the non-ASCII name must round-trip exactly")
	assert.Equal(t, name, rows[0]["Name"])
}

// Ordering comparisons go through the same parameter set as the rest, so verify one
// against real cmdlet output instead of only asserting on the generated text.
func TestRunCmdletOrderingComparison(t *testing.T) {
	requirePowerShell(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), make([]byte, 4096), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "small.txt"), []byte("x"), 0o600))

	c := newExecTestCheck("Get-ChildItem")
	rows, err := c.runCmdlet("Get-ChildItem",
		[]parameterEntry{{Name: "Path", Value: dir}},
		[]whereEntry{{Property: "Length", Op: "gt", Value: 1024}},
		[]string{"Name"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "big.txt", rows[0]["Name"])
}

// End-to-end proof that a filter value reaches the cmdlet as data and can never
// reach a code position, even when it is shaped to look like PowerShell.
func TestRunCmdletBindsHostileWhereValueAsData(t *testing.T) {
	requirePowerShell(t)

	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "pwned.txt"))
	hostile := "Running’ -or $(New-Item -Path " + marker + " -ItemType File -Force) -or ’Running"

	c := newExecTestCheck("Get-Service")
	rows, err := c.runCmdlet("Get-Service", nil,
		[]whereEntry{{Property: "Status", Op: "eq", Value: hostile}},
		[]string{"Name"})
	require.NoError(t, err)
	assert.Empty(t, rows, "no service status equals the hostile string")
	assert.NoFileExists(t, marker, "the payload must never execute")
}
