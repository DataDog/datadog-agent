// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

package testutil

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	nvidiaSmi           = "nvidia-smi"
	standardMetricCount = 12
	GrEngineActiveID    = "1001"
)

type SmiSample struct {
	Index            int
	PowerWatts       *float64
	GPUTempC         *float64
	MemTempC         *float64
	SMUtilPct        *float64
	MemUtilPct       *float64
	EncoderPct       *float64
	DecoderPct       *float64
	JPEGPct          *float64
	OFAPct           *float64
	MemClockMHz      *float64
	ProcClockMHz     *float64
	GraphicsActivity *float64
}

func RequireSmi(t *testing.T) {
	_, err := exec.LookPath(nvidiaSmi)
	require.NoError(t, err)
}

func CollectSmiSample(t *testing.T, deviceID string) *SmiSample {
	gpmMetrics := []string{
		"1", // Graphics Activity
	}
	// GPM metrics are a delta between consecutive samples, so the first dmon
	// cycle always reports "-". Run multiple cycles and read a later line.
	args := []string{"dmon", "--id", deviceID, "-c", "3", "--format", "csv,noheader,nounit"}
	if len(gpmMetrics) > 0 {
		args = append(args, "--gpm-metrics", strings.Join(gpmMetrics, ","))
	}
	cmd := exec.Command(nvidiaSmi, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nvidia-smi failed (%v):\nstderr: %s", err, ee.Stderr)
		}
		require.NoError(t, err, "could not collect sample")
	}

	// One data line per monitoring cycle (single device via --id). Read the
	// third line so the GPM metrics have two prior samples to diff against.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.GreaterOrEqual(t, len(lines), 3,
		"expected at least 3 sample lines, got %d:\n%s", len(lines), string(out))
	values := strings.Split(strings.TrimSpace(lines[2]), ",")

	require.Equal(t, len(values), standardMetricCount+len(gpmMetrics), "invalid output")
	idx, err := strconv.Atoi(strings.TrimSpace(values[0]))
	require.NoError(t, err, "bad gpu index")
	gpmValues := make([]*float64, len(gpmMetrics))
	if len(gpmMetrics) > 0 {
		require.Equal(t, standardMetricCount+len(gpmMetrics), len(values), "invalid output")
		for i := standardMetricCount; i < len(values); i++ {
			gpmValues[i-standardMetricCount] = parseFloatField(values[i])
		}
	}
	return &SmiSample{
		Index:            idx,
		PowerWatts:       parseFloatField(values[1]),
		GPUTempC:         parseFloatField(values[2]),
		MemTempC:         parseFloatField(values[3]),
		SMUtilPct:        parseFloatField(values[4]),
		MemUtilPct:       parseFloatField(values[5]),
		EncoderPct:       parseFloatField(values[6]),
		DecoderPct:       parseFloatField(values[7]),
		JPEGPct:          parseFloatField(values[8]),
		OFAPct:           parseFloatField(values[9]),
		MemClockMHz:      parseFloatField(values[10]),
		ProcClockMHz:     parseFloatField(values[11]),
		GraphicsActivity: parseFloatField(values[12]),
	}
}

// parseFloatField returns nil for "-" or unparseable values.
func parseFloatField(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "-" || s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}
