// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
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
