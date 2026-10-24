// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ps

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procinfo"
)

func TestParseOutputColumnsSupportsRepeatedCommaAndSpaceLists(t *testing.T) {
	columns, err := parseOutputColumns([]string{"pid,ppid", "uid state", "pcpu,%mem,comm"})
	require.NoError(t, err)
	require.Equal(t, []outputField{
		fieldPID,
		fieldPPID,
		fieldUID,
		fieldState,
		fieldPCPU,
		fieldPMem,
		fieldComm,
	}, columns)
}

func TestFormatAndSortAliases(t *testing.T) {
	columns, err := parseOutputColumns([]string{"%CPU,%MEM"})
	require.NoError(t, err)
	require.Equal(t, []outputField{
		fieldPCPU,
		fieldPMem,
	}, columns)

	keys, err := parseSortKeys("-%CPU +%MEM")
	require.NoError(t, err)
	require.Equal(t, []sortKey{
		{field: fieldPCPU, descending: true},
		{field: fieldPMem},
	}, keys)
}

func TestParseOutputColumnsRejectsUnsafeAndMalformedFields(t *testing.T) {
	tests := []struct {
		name    string
		formats []string
	}{
		{name: "argv alias", formats: []string{"args"}},
		{name: "command alias", formats: []string{"command"}},
		{name: "empty", formats: []string{""}},
		{name: "empty segment", formats: []string{"pid,,comm"}},
		{name: "custom header", formats: []string{"pid=PID"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseOutputColumns(test.formats)
			require.Error(t, err)
		})
	}
}

func TestParseSortKeys(t *testing.T) {
	keys, err := parseSortKeys("-rss,+pid comm")
	require.NoError(t, err)
	require.Equal(t, []sortKey{
		{field: fieldRSS, descending: true},
		{field: fieldPID},
		{field: fieldComm},
	}, keys)
}

func TestParseSortKeysRejectsMalformedSpecs(t *testing.T) {
	for _, spec := range []string{"", "rss,", "rss,,pid", "-", "+", "args"} {
		t.Run(spec, func(t *testing.T) {
			_, err := parseSortKeys(spec)
			require.Error(t, err)
		})
	}
}

func TestRequestedMetricsIncludesColumnsSortAndLegacyFields(t *testing.T) {
	columns := []outputField{fieldRSS, fieldETime}
	keys := []sortKey{{field: fieldPCPU}}
	require.Equal(
		t,
		procinfo.MetricRSS|procinfo.MetricElapsed|procinfo.MetricPCPU,
		requestedMetrics(columns, keys, false),
	)

	require.Equal(t, procinfo.MetricCPUTime, requestedMetrics(nil, nil, false))
	require.Equal(
		t,
		procinfo.MetricCPUTime|procinfo.MetricPCPU|procinfo.MetricStartTime,
		requestedMetrics(nil, nil, true),
	)
}

func TestRequestedMetricsDoesNotAddHiddenDependencies(t *testing.T) {
	tests := []struct {
		name   string
		field  outputField
		metric procinfo.Metrics
	}{
		{name: "pcpu", field: fieldPCPU, metric: procinfo.MetricPCPU},
		{name: "pmem", field: fieldPMem, metric: procinfo.MetricPMem},
		{name: "etime", field: fieldETime, metric: procinfo.MetricElapsed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(
				t,
				test.metric,
				requestedMetrics([]outputField{test.field}, nil, false),
			)
			require.Equal(
				t,
				test.metric,
				requestedMetrics(
					[]outputField{fieldPID},
					[]sortKey{{field: test.field}},
					false,
				),
			)
		})
	}
}

func TestSortProcsUsesRawValuesAndPlacesUnavailableLast(t *testing.T) {
	procs := []procinfo.ProcInfo{
		{PID: 3},
		{PID: 2, RSSKiB: 200, Available: procinfo.MetricRSS},
		{PID: 1, RSSKiB: 200, Available: procinfo.MetricRSS},
		{PID: 4, RSSKiB: 100, Available: procinfo.MetricRSS},
	}

	sortProcs(procs, []sortKey{
		{field: fieldRSS, descending: true},
		{field: fieldPID},
	})

	require.Equal(t, []int{1, 2, 4, 3}, []int{
		procs[0].PID,
		procs[1].PID,
		procs[2].PID,
		procs[3].PID,
	})
}

func TestSortProcsCoversFieldTypesAndDirections(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name  string
		field outputField
		low   procinfo.ProcInfo
		high  procinfo.ProcInfo
	}{
		{
			name:  "numeric uid",
			field: fieldUID,
			low:   procinfo.ProcInfo{UID: "2"},
			high:  procinfo.ProcInfo{UID: "10"},
		},
		{
			name:  "string",
			field: fieldComm,
			low:   procinfo.ProcInfo{Cmd: "alpha"},
			high:  procinfo.ProcInfo{Cmd: "zulu"},
		},
		{
			name:  "start time",
			field: fieldSTime,
			low: procinfo.ProcInfo{
				StartTime: start,
				Available: procinfo.MetricStartTime,
			},
			high: procinfo.ProcInfo{
				StartTime: start.Add(time.Hour),
				Available: procinfo.MetricStartTime,
			},
		},
		{
			name:  "cpu time",
			field: fieldTime,
			low: procinfo.ProcInfo{
				CPUTime:   time.Second,
				Available: procinfo.MetricCPUTime,
			},
			high: procinfo.ProcInfo{
				CPUTime:   10 * time.Second,
				Available: procinfo.MetricCPUTime,
			},
		},
		{
			name:  "unsigned memory",
			field: fieldRSS,
			low: procinfo.ProcInfo{
				RSSKiB:    2,
				Available: procinfo.MetricRSS,
			},
			high: procinfo.ProcInfo{
				RSSKiB:    10,
				Available: procinfo.MetricRSS,
			},
		},
		{
			name:  "floating point percentage",
			field: fieldPCPU,
			low: procinfo.ProcInfo{
				PCPU:      2.5,
				Available: procinfo.MetricPCPU,
			},
			high: procinfo.ProcInfo{
				PCPU:      10.5,
				Available: procinfo.MetricPCPU,
			},
		},
		{
			name:  "elapsed duration",
			field: fieldETime,
			low: procinfo.ProcInfo{
				Elapsed:   time.Second,
				Available: procinfo.MetricElapsed,
			},
			high: procinfo.ProcInfo{
				Elapsed:   10 * time.Second,
				Available: procinfo.MetricElapsed,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			low := test.low
			low.PID = 1
			high := test.high
			high.PID = 2
			unavailable := procinfo.ProcInfo{PID: 3}

			ascending := []procinfo.ProcInfo{unavailable, high, low}
			sortProcs(ascending, []sortKey{{field: test.field}})
			require.Equal(t, []int{1, 2, 3}, procPIDs(ascending))

			descending := []procinfo.ProcInfo{unavailable, low, high}
			sortProcs(descending, []sortKey{{field: test.field, descending: true}})
			require.Equal(t, []int{2, 1, 3}, procPIDs(descending))
		})
	}
}

func TestSortProcsUsesLexicalFallbackForMixedUIDs(t *testing.T) {
	procs := []procinfo.ProcInfo{
		{PID: 1, UID: "root"},
		{PID: 2, UID: "10"},
		{PID: 3},
	}

	sortProcs(procs, []sortKey{{field: fieldUID}})

	require.Equal(t, []int{2, 1, 3}, procPIDs(procs))
}

func TestSortProcsIsStableForEqualKeys(t *testing.T) {
	procs := []procinfo.ProcInfo{
		{PID: 3, RSSKiB: 10, Available: procinfo.MetricRSS},
		{PID: 1, RSSKiB: 10, Available: procinfo.MetricRSS},
		{PID: 2, RSSKiB: 10, Available: procinfo.MetricRSS},
	}

	sortProcs(procs, []sortKey{{field: fieldRSS}})

	require.Equal(t, []int{3, 1, 2}, procPIDs(procs))
}

func procPIDs(procs []procinfo.ProcInfo) []int {
	pids := make([]int, len(procs))
	for i := range procs {
		pids[i] = procs[i].PID
	}
	return pids
}

func TestFormatFieldDoesNotFabricateUnavailableMetrics(t *testing.T) {
	proc := procinfo.ProcInfo{
		STime:   "12:34",
		Time:    "00:01:02",
		RSSKiB:  1024,
		VSZKiB:  2048,
		PMem:    3.2,
		PCPU:    12.5,
		Elapsed: time.Minute,
	}
	for _, field := range []outputField{
		fieldSTime,
		fieldTime,
		fieldRSS,
		fieldVSZ,
		fieldPMem,
		fieldPCPU,
		fieldETime,
	} {
		require.Equal(t, "-", formatField(proc, field), fieldDefinitions[field].name)
	}
}

func TestFormatFieldCoversEverySupportedField(t *testing.T) {
	proc := procinfo.ProcInfo{
		PID:     42,
		PPID:    1,
		UID:     "1000",
		State:   "R",
		TTY:     "pts/2",
		STime:   "12:34",
		Time:    "00:01:02",
		Cmd:     "worker",
		RSSKiB:  1024,
		VSZKiB:  2048,
		PMem:    3.2,
		PCPU:    12.5,
		Elapsed: 25*time.Hour + 2*time.Minute + 3*time.Second,
		Available: procinfo.MetricStartTime | procinfo.MetricCPUTime | procinfo.MetricElapsed |
			procinfo.MetricRSS | procinfo.MetricVSZ | procinfo.MetricPMem | procinfo.MetricPCPU,
	}

	tests := []struct {
		field outputField
		want  string
	}{
		{field: fieldPID, want: "42"},
		{field: fieldPPID, want: "1"},
		{field: fieldUID, want: "1000"},
		{field: fieldState, want: "R"},
		{field: fieldTTY, want: "pts/2"},
		{field: fieldSTime, want: "12:34"},
		{field: fieldTime, want: "00:01:02"},
		{field: fieldComm, want: "worker"},
		{field: fieldRSS, want: "1024"},
		{field: fieldVSZ, want: "2048"},
		{field: fieldPMem, want: "3.2"},
		{field: fieldPCPU, want: "12.5"},
		{field: fieldETime, want: "1-01:02:03"},
	}
	for _, test := range tests {
		require.Equal(t, test.want, formatField(proc, test.field), fieldDefinitions[test.field].name)
	}
}

func TestFormatFieldRejectsInvalidDerivedValues(t *testing.T) {
	proc := procinfo.ProcInfo{
		PMem:      math.NaN(),
		PCPU:      math.Inf(1),
		Elapsed:   -time.Second,
		Available: procinfo.MetricPMem | procinfo.MetricPCPU | procinfo.MetricElapsed,
	}

	require.Equal(t, "-", formatField(proc, fieldPMem))
	require.Equal(t, "-", formatField(proc, fieldPCPU))
	require.Equal(t, "-", formatField(proc, fieldETime))
}

func TestPrintCustomProcsUsesDeterministicAlignment(t *testing.T) {
	var stdout bytes.Buffer
	printCustomProcs(
		&builtins.CallContext{Stdout: &stdout},
		[]procinfo.ProcInfo{
			{PID: 2, UID: "1000", RSSKiB: 9, Cmd: "z", Available: procinfo.MetricRSS},
			{PID: 123, UID: "7", RSSKiB: 100, Cmd: "worker", Available: procinfo.MetricRSS},
		},
		[]outputField{
			fieldPID,
			fieldUID,
			fieldRSS,
			fieldComm,
		},
	)

	require.Equal(t,
		"PID UID  RSS COMMAND\n"+
			"  2 1000   9 z\n"+
			"123 7    100 worker\n",
		stdout.String(),
	)
}

func TestFullFormatDoesNotFabricateUnavailablePCPU(t *testing.T) {
	var stdout bytes.Buffer
	printProcs(&builtins.CallContext{Stdout: &stdout}, []procinfo.ProcInfo{{
		UID:   "1000",
		PID:   42,
		PPID:  1,
		STime: "-",
		TTY:   "?",
		Time:  "-",
		Cmd:   "worker",
	}}, true)

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "-", strings.Fields(lines[1])[3])
}

func TestFormatElapsed(t *testing.T) {
	require.Equal(t, "00:09", formatElapsed(9*time.Second))
	require.Equal(t, "02:03", formatElapsed(2*time.Minute+3*time.Second))
	require.Equal(t, "04:05:06", formatElapsed(4*time.Hour+5*time.Minute+6*time.Second))
	require.Equal(t, "2-04:05:06", formatElapsed(52*time.Hour+5*time.Minute+6*time.Second))
}
