// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ebpfcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
)

func newTestCheck() *EBPFCheck {
	return newCheck().(*EBPFCheck)
}

// decodedLine is a series log line decoded back from JSON.
type decodedLine struct {
	Message   string             `json:"message"`
	Timestamp int64              `json:"timestamp"`
	Tags      map[string]string  `json:"tags"`
	Values    map[string]float64 `json:"values"`
	Types     map[string]string  `json:"types"`
}

// sortSeries orders series by identity, since collectSeries returns them in
// map iteration order.
func sortSeries(allSeries []*series) []*series {
	return slices.SortedFunc(slices.Values(allSeries), func(a, b *series) int {
		return strings.Compare(a.key, b.key)
	})
}

// decodeLines marshals the series ordered by identity and decodes each line.
func decodeLines(t *testing.T, allSeries []*series) []decodedLine {
	t.Helper()
	now := time.UnixMilli(1792494774184)
	lines := make([]decodedLine, 0, len(allSeries))
	for _, s := range sortSeries(allSeries) {
		data, err := s.marshal(now)
		require.NoError(t, err)
		var l decodedLine
		require.NoError(t, json.Unmarshal(data, &l))
		lines = append(lines, l)
	}
	return lines
}

// linesWithTag returns the lines whose tags contain key.
func linesWithTag(lines []decodedLine, key string) []decodedLine {
	var out []decodedLine
	for _, l := range lines {
		if _, ok := l.Tags[key]; ok {
			out = append(out, l)
		}
	}
	return out
}

// moduleLines returns the lines whose only identity tag is module.
func moduleLines(lines []decodedLine) map[string]decodedLine {
	out := make(map[string]decodedLine)
	for _, l := range lines {
		if len(l.Tags) == 1 && l.Tags["module"] != "" {
			out[l.Tags["module"]] = l
		}
	}
	return out
}

func TestMapSeries(t *testing.T) {
	c := newTestCheck()
	stats := ebpfcheck.EBPFStats{
		Maps: []ebpfcheck.EBPFMapStats{{
			Name:       "lock_stats",
			Type:       "Hash",
			Module:     "lock-contention",
			MaxSize:    157728,
			RSS:        157728,
			MaxEntries: 1188,
			Entries:    7,
			NoPrealloc: false,
		}},
	}

	maps := linesWithTag(decodeLines(t, c.collectSeries(stats)), "map_name")
	require.Len(t, maps, 1)
	l := maps[0]

	assert.Equal(t, seriesMessage, l.Message)
	assert.Equal(t, int64(1792494774184), l.Timestamp)
	assert.Equal(t, map[string]string{
		"map_name":    "lock_stats",
		"map_type":    "Hash",
		"module":      "lock-contention",
		"no_prealloc": "false",
	}, l.Tags)
	assert.Equal(t, map[string]float64{
		"ebpf_maps_memory_max":          157728,
		"ebpf_maps_memory_rss":          157728,
		"ebpf_maps_max_entries":         1188,
		"ebpf_maps_entry_count":         7,
		"ebpf_maps_occupation":          7.0 / 1188,
		"ebpf_maps_occupation_increase": 7.0 / 1188,
	}, l.Values)
	for k, typ := range l.Types {
		assert.Equal(t, "gauge", typ, k)
	}
	assert.Len(t, l.Types, len(l.Values))
}

func TestMapSeriesNoOccupationWithoutMaxEntries(t *testing.T) {
	c := newTestCheck()
	stats := ebpfcheck.EBPFStats{
		Maps: []ebpfcheck.EBPFMapStats{{Name: "ringbuf", Type: "RingBuf", Module: "cws", MaxSize: 4096, Entries: 0}},
	}

	maps := linesWithTag(decodeLines(t, c.collectSeries(stats)), "map_name")
	require.Len(t, maps, 1)
	assert.NotContains(t, maps[0].Values, "ebpf_maps_occupation")
	assert.NotContains(t, maps[0].Values, "ebpf_maps_occupation_increase")
	assert.NotContains(t, maps[0].Tags, "no_prealloc")
}

func TestTotalsSeries(t *testing.T) {
	c := newTestCheck()
	stats := ebpfcheck.EBPFStats{
		Maps: []ebpfcheck.EBPFMapStats{
			{Name: "a1", Type: "Array", Module: "mod_a", MaxSize: 100, RSS: 10, MaxEntries: 1, Entries: -1},
			{Name: "b1", Type: "Array", Module: "mod_b", MaxSize: 200, RSS: 20, MaxEntries: 1, Entries: -1},
		},
		Programs: []ebpfcheck.EBPFProgramStats{
			{Name: "p1", Type: "Kprobe", Module: "mod_a", RSS: 4096, XlatedProgLen: 160, VerifiedInsns: 18},
		},
	}

	lines := decodeLines(t, c.collectSeries(stats))

	var global []decodedLine
	for _, l := range lines {
		if len(l.Tags) == 0 {
			global = append(global, l)
		}
	}
	require.Len(t, global, 1)
	assert.NotNil(t, global[0].Tags, "global tags must be an empty object, not null")
	assert.Equal(t, map[string]float64{
		"ebpf_maps_memory_max_total":     300,
		"ebpf_maps_memory_rss_total":     30,
		"ebpf_programs_memory_rss_total": 4096,
	}, global[0].Values)

	modules := moduleLines(lines)
	require.Len(t, modules, 2)
	// Map and program totals for one module share a line.
	assert.Equal(t, map[string]float64{
		"ebpf_maps_memory_max_permodule_total":                     100,
		"ebpf_maps_memory_rss_permodule_total":                     10,
		"ebpf_programs_memory_rss_permodule_total":                 4096,
		"ebpf_programs_xlated_instruction_len_permodule_total":     160,
		"ebpf_programs_verified_instruction_count_permodule_total": 18,
	}, modules["mod_a"].Values)
	assert.Equal(t, map[string]float64{
		"ebpf_maps_memory_max_permodule_total": 200,
		"ebpf_maps_memory_rss_permodule_total": 20,
	}, modules["mod_b"].Values)
}

func TestProgramSeries(t *testing.T) {
	c := newTestCheck()
	stats := ebpfcheck.EBPFStats{
		Programs: []ebpfcheck.EBPFProgramStats{
			{Name: "hook_security_path_mkdir", Type: "Kprobe", Module: "cws", XlatedProgLen: 160, VerifiedInsns: 18, RSS: 4096, RunCount: 10, Runtime: 500},
			{Name: "tcp_sendmsg", Type: "Kprobe", Module: "network_tracer", XlatedProgLen: 80, VerifiedInsns: 9, RSS: 4096, RunCount: 3},
		},
		KprobeStats: []ebpfcheck.KprobeStats{
			{Name: "hook_security_path_mkdir", Type: "Kprobe", Module: "cws", KprobeHits: 10},
		},
	}

	programs := linesWithTag(decodeLines(t, c.collectSeries(stats)), "program_name")
	require.Len(t, programs, 2)

	mkdir := programs[0]
	assert.Equal(t, map[string]string{
		"program_name": "hook_security_path_mkdir",
		"program_type": "Kprobe",
		"module":       "cws",
	}, mkdir.Tags)
	assert.Equal(t, map[string]float64{
		"ebpf_programs_xlated_instruction_len":     160,
		"ebpf_programs_verified_instruction_count": 18,
		"ebpf_programs_memory_rss":                 4096,
		"ebpf_programs_run_count":                  10,
		"ebpf_programs_runtime_ns":                 500,
		"ebpf_kprobes_kprobe_hits":                 10,
	}, mkdir.Values)
	assert.Equal(t, map[string]string{
		"ebpf_programs_xlated_instruction_len":     "gauge",
		"ebpf_programs_verified_instruction_count": "gauge",
		"ebpf_programs_memory_rss":                 "gauge",
		"ebpf_programs_run_count":                  "count",
		"ebpf_programs_runtime_ns":                 "count",
		"ebpf_kprobes_kprobe_hits":                 "count",
	}, mkdir.Types)

	assert.Equal(t, "tcp_sendmsg", programs[1].Tags["program_name"])
}

func TestCountsAreDeltas(t *testing.T) {
	c := newTestCheck()
	runCount := func(n uint64) float64 {
		stats := ebpfcheck.EBPFStats{
			Programs: []ebpfcheck.EBPFProgramStats{{Name: "p", Type: "Kprobe", Module: "cws", RunCount: n}},
		}
		programs := linesWithTag(decodeLines(t, c.collectSeries(stats)), "program_name")
		require.Len(t, programs, 1)
		return programs[0].Values["ebpf_programs_run_count"]
	}

	assert.Equal(t, 10.0, runCount(10), "first reading is reported as-is")
	assert.Equal(t, 15.0, runCount(25), "later readings report the increase")
	assert.Equal(t, 0.0, runCount(25), "no increase reports zero")
	assert.Equal(t, 5.0, runCount(5), "a lower reading is a reset, reported as-is")
	assert.Equal(t, 2.0, runCount(7))
}

func TestCountStateDropsVanishedPrograms(t *testing.T) {
	c := newTestCheck()
	c.collectSeries(ebpfcheck.EBPFStats{
		Programs: []ebpfcheck.EBPFProgramStats{{Name: "p", Type: "Kprobe", Module: "cws", RunCount: 10}},
	})
	require.Len(t, c.previousCounts, 1)

	c.collectSeries(ebpfcheck.EBPFStats{})
	assert.Empty(t, c.previousCounts)
}

func TestSeriesOutputIsDeterministic(t *testing.T) {
	stats := ebpfcheck.EBPFStats{
		Maps: []ebpfcheck.EBPFMapStats{
			{Name: "m2", Type: "Hash", Module: "mod_b", MaxSize: 1, MaxEntries: 1, Entries: 1},
			{Name: "m1", Type: "Hash", Module: "mod_a", MaxSize: 1, MaxEntries: 1, Entries: 1},
		},
		Programs: []ebpfcheck.EBPFProgramStats{
			{Name: "p2", Type: "Kprobe", Module: "mod_b", RSS: 1},
			{Name: "p1", Type: "Kprobe", Module: "mod_a", RSS: 1},
		},
	}
	render := func() []string {
		now := time.UnixMilli(0)
		var out []string
		for _, s := range sortSeries(newTestCheck().collectSeries(stats)) {
			data, err := s.marshal(now)
			require.NoError(t, err)
			out = append(out, string(data))
		}
		return out
	}

	first := render()
	for range 10 {
		assert.Equal(t, first, render())
	}
}

func TestSeriesValuesAreNumbersWithoutDottedKeys(t *testing.T) {
	c := newTestCheck()
	stats := ebpfcheck.EBPFStats{
		Maps:     []ebpfcheck.EBPFMapStats{{Name: "m", Type: "Hash", Module: "mod", MaxSize: 1, MaxEntries: 4, Entries: 2}},
		Programs: []ebpfcheck.EBPFProgramStats{{Name: "p", Type: "Kprobe", Module: "mod", RSS: 1, RunCount: 1}},
	}

	for _, s := range c.collectSeries(stats) {
		data, err := s.marshal(time.Now())
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		values, ok := raw["values"].(map[string]any)
		require.True(t, ok)
		types, ok := raw["types"].(map[string]any)
		require.True(t, ok)
		require.Len(t, types, len(values))
		for k, v := range values {
			assert.NotContains(t, k, ".")
			assert.IsType(t, float64(0), v, k)
			assert.Contains(t, types, k)
		}
		assert.False(t, strings.Contains(string(data), `"ebpf.`), "no dotted metric names in %s", data)
	}
}

func TestSeriesAddMergesAndKeepsFirstConflictingTag(t *testing.T) {
	s := newSeriesSet().get(map[string]string{"map_name": "m"})
	s.add("a", metricTypeGauge, 1, map[string]string{"extra": "first"})
	s.add("b", metricTypeGauge, 2, map[string]string{"extra": "second"})
	s.add("a", metricTypeGauge, 3, nil)

	assert.Equal(t, map[string]string{"map_name": "m", "extra": "first"}, s.tags)
	assert.Equal(t, map[string]float64{"a": 3, "b": 2}, s.values)
}
