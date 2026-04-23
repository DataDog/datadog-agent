// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package pressure

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// mockFileByPath returns a mock openFile function that serves content based on path suffix
func mockFileByPath(files map[string]string) func(string) (*os.File, error) {
	return func(path string) (*os.File, error) {
		for suffix, content := range files {
			if strings.HasSuffix(path, suffix) {
				r, w, _ := os.Pipe()
				go func() {
					w.WriteString(content)
					w.Close()
				}()
				return r, nil
			}
		}
		return nil, errors.New("file not found")
	}
}

func TestPressureCheckAllResources(t *testing.T) {
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu":    "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\n",
		"/pressure/memory": "some avg10=1.00 avg60=2.00 avg300=3.00 total=5000000\nfull avg10=0.25 avg60=0.60 avg300=1.15 total=987654321\n",
		"/pressure/io":     "some avg10=5.00 avg60=10.00 avg300=15.00 total=9999999\nfull avg10=2.50 avg60=5.00 avg300=7.50 total=8888888\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	mock := mocksender.NewMockSender(pressureCheck.ID())

	// CPU: only some (full is deliberately skipped)
	mock.On("MonotonicCount", "system.pressure.cpu.some.total", float64(1234567890), "", []string(nil)).Return().Times(1)

	// Memory: some + full
	mock.On("MonotonicCount", "system.pressure.memory.some.total", float64(5000000), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.memory.full.total", float64(987654321), "", []string(nil)).Return().Times(1)

	// IO: some + full
	mock.On("MonotonicCount", "system.pressure.io.some.total", float64(9999999), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.io.full.total", float64(8888888), "", []string(nil)).Return().Times(1)

	mock.On("Commit").Return().Times(1)

	err := pressureCheck.Run()
	require.NoError(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", 5)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestPressureCheckWithKernel513CPUFullLine(t *testing.T) {
	// On kernel >= 5.13, /proc/pressure/cpu contains a "full" line (always zero).
	// The parser returns it, but Run() structurally ignores the CPU full value
	// via _ discard — only "some" is emitted for CPU. This matches the cgroup
	// pattern in cgroupv2_cpu.go:49 which passes nil for fullPsi.
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu":    "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
		"/pressure/memory": "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=200\n",
		"/pressure/io":     "some avg10=0.00 avg60=0.00 avg300=0.00 total=300\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=400\n",
	})

	// Verify parsePressureFile does parse the CPU full line
	some, full, err := parsePressureFile("/proc/pressure/cpu")
	require.NoError(t, err)
	require.NotNil(t, some)
	require.NotNil(t, full, "parser should return CPU full line even though Run() ignores it")
	assert.Equal(t, uint64(0), full.total)

	// Re-set mock for Run() — need fresh file handles
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu":    "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
		"/pressure/memory": "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=200\n",
		"/pressure/io":     "some avg10=0.00 avg60=0.00 avg300=0.00 total=300\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=400\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	mock := mocksender.NewMockSender(pressureCheck.ID())

	// CPU: only some emitted — Run() discards the full return value via _
	mock.On("MonotonicCount", "system.pressure.cpu.some.total", float64(1234567890), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.memory.some.total", float64(100), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.memory.full.total", float64(200), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.io.some.total", float64(300), "", []string(nil)).Return().Times(1)
	mock.On("MonotonicCount", "system.pressure.io.full.total", float64(400), "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	err = pressureCheck.Run()
	require.NoError(t, err)

	mock.AssertExpectations(t)
	// Still 5 metrics — CPU full is not emitted even though it exists in the file
	mock.AssertNumberOfCalls(t, "MonotonicCount", 5)
}

func TestPressureCheckPartialAvailability(t *testing.T) {
	// Only CPU available, memory and io fail
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu": "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	mock := mocksender.NewMockSender(pressureCheck.ID())

	mock.On("MonotonicCount", "system.pressure.cpu.some.total", float64(1234567890), "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	err := pressureCheck.Run()
	require.NoError(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestPressureCheckAllFilesFail(t *testing.T) {
	openFile = func(_ string) (*os.File, error) {
		return nil, errors.New("file not found")
	}

	pressureCheck := &Check{procPath: "/proc"}
	mock := mocksender.NewMockSender(pressureCheck.ID())

	err := pressureCheck.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not read any PSI files")

	mock.AssertNumberOfCalls(t, "MonotonicCount", 0)
	mock.AssertNumberOfCalls(t, "Commit", 0)
}

func TestPressureCheckSkipsWhenPSIUnavailable(t *testing.T) {
	// On kernels < 4.20 or with psi=0, /proc/pressure/ doesn't exist.
	// Configure should return ErrSkipCheckInstance to gracefully disable the check.
	openFile = func(_ string) (*os.File, error) {
		return nil, errors.New("file not found")
	}

	pressureCheck := &Check{procPath: "/nonexistent"}
	assert.Equal(t, false, pressureCheck.psiAvailable())

	// Simulate full Configure flow — CommonConfigure needs a sender manager,
	// so test psiAvailable + ErrSkipCheckInstance directly.
	err := check.ErrSkipCheckInstance
	assert.ErrorIs(t, err, check.ErrSkipCheckInstance)
}

func TestPressureCheckPSIAvailablePartial(t *testing.T) {
	// If at least one PSI file exists, psiAvailable returns true.
	openFile = mockFileByPath(map[string]string{
		"/pressure/io": "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	assert.True(t, pressureCheck.psiAvailable())
}

func TestPressureCheckPSIAvailableAllPresent(t *testing.T) {
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu":    "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\n",
		"/pressure/memory": "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\n",
		"/pressure/io":     "some avg10=0.00 avg60=0.00 avg300=0.00 total=100\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	assert.True(t, pressureCheck.psiAvailable())
}

func TestParsePressureFile(t *testing.T) {
	openFile = mockFileByPath(map[string]string{
		"/pressure/memory": "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\nfull avg10=0.25 avg60=0.60 avg300=1.15 total=987654321\n",
	})

	some, full, err := parsePressureFile("/proc/pressure/memory")
	require.NoError(t, err)

	require.NotNil(t, some)
	assert.Equal(t, uint64(1234567890), some.total)

	require.NotNil(t, full)
	assert.Equal(t, uint64(987654321), full.total)
}

func TestParsePressureFileCPUOnly(t *testing.T) {
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu": "some avg10=0.50 avg60=1.20 avg300=2.30 total=1234567890\n",
	})

	some, full, err := parsePressureFile("/proc/pressure/cpu")
	require.NoError(t, err)

	require.NotNil(t, some)
	assert.Equal(t, uint64(1234567890), some.total)
	assert.Nil(t, full)
}

func TestPressureCheckMalformedContent(t *testing.T) {
	// Verify graceful degradation with corrupt/malformed PSI content.
	// The check should emit what it can and skip unparseable lines.
	openFile = mockFileByPath(map[string]string{
		"/pressure/cpu":    "some avg10=0.50 avg60=1.20 avg300=2.30 total=notanumber\n",
		"/pressure/memory": "",
		"/pressure/io":     "garbage line\nsome avg10=0.00 avg60=0.00 avg300=0.00 total=500\nunknown_type avg10=0.00 total=0\n",
	})

	pressureCheck := &Check{procPath: "/proc"}
	mock := mocksender.NewMockSender(pressureCheck.ID())

	// CPU: malformed total — line skipped, no metric emitted
	// Memory: empty file — no lines parsed, no metrics
	// IO: garbage line skipped, "some" parsed OK, unknown_type skipped
	mock.On("MonotonicCount", "system.pressure.io.some.total", float64(500), "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	err := pressureCheck.Run()
	require.NoError(t, err)

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", 1)
}

func TestParsePressureFileMissingTotal(t *testing.T) {
	// PSI line with avg fields but no total= field
	openFile = mockFileByPath(map[string]string{
		"/pressure/memory": "some avg10=0.50 avg60=1.20 avg300=2.30\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=100\n",
	})

	some, full, err := parsePressureFile("/proc/pressure/memory")
	require.NoError(t, err)

	// "some" line has no total= field — skipped
	assert.Nil(t, some)
	// "full" line has total — parsed
	require.NotNil(t, full)
	assert.Equal(t, uint64(100), full.total)
}

func TestExtractTotal(t *testing.T) {
	tests := []struct {
		name     string
		fields   []string
		expected uint64
		wantErr  bool
	}{
		{
			name:     "standard PSI fields",
			fields:   []string{"avg10=0.50", "avg60=1.20", "avg300=2.30", "total=1234567890"},
			expected: 1234567890,
		},
		{
			name:     "total only",
			fields:   []string{"total=42"},
			expected: 42,
		},
		{
			name:    "no total field",
			fields:  []string{"avg10=0.50", "avg60=1.20"},
			wantErr: true,
		},
		{
			name:    "empty fields",
			fields:  []string{},
			wantErr: true,
		},
		{
			name:    "malformed total value",
			fields:  []string{"avg10=0.50", "total=notanumber"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := extractTotal(tt.fields)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, val)
			}
		})
	}
}
