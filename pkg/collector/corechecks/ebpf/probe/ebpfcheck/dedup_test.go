// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpfcheck

import (
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
)

func TestProgramAggregator(t *testing.T) {
	// the instruction length tells which program an entry was taken from
	classifier := func(id uint32, runtime time.Duration, runCount, recursionMisses uint64) model.EBPFProgramStats {
		return model.EBPFProgramStats{ID: id, Name: "classifier_egress", Type: "SchedCLS", Module: "cws", XlatedProgLen: id, Runtime: runtime, RunCount: runCount, RecursionMisses: recursionMisses}
	}
	kprobe := func(id uint32, runtime time.Duration, runCount uint64) model.EBPFProgramStats {
		return model.EBPFProgramStats{ID: id, Name: "kprobe__tcp_close", Type: "Kprobe", Module: "npm_tracer", XlatedProgLen: id, Runtime: runtime, RunCount: runCount}
	}
	classifierKey := programKey{name: "classifier_egress", typ: "SchedCLS", module: "cws"}
	kprobeKey := programKey{name: "kprobe__tcp_close", typ: "Kprobe", module: "npm_tracer"}

	agg := newProgramAggregator()
	collect := func(programs ...model.EBPFProgramStats) (model.EBPFStats, map[programKey]ebpf.ProgramID) {
		var stats model.EBPFStats
		uniquePrograms := make(map[programKey]ebpf.ProgramID)
		for i := range programs {
			agg.add(&stats, uniquePrograms, &programs[i])
		}
		agg.flush(&stats, uniquePrograms)
		return stats, uniquePrograms
	}

	// the template of the clones has the lowest ID and never runs
	stats, uniquePrograms := collect(classifier(10, 0, 0, 0), classifier(11, 100, 10, 1), classifier(12, 50, 5, 0), kprobe(20, 7, 1))
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 150, 15, 1), kprobe(20, 7, 1)}, stats.Programs)
	assert.Equal(t, map[programKey]ebpf.ProgramID{classifierKey: 10, kprobeKey: 20}, uniquePrograms)

	// clone 11 is unloaded, clone 12 runs 3 more times and clone 13 is new
	stats, _ = collect(classifier(10, 0, 0, 0), classifier(12, 80, 8, 0), classifier(13, 20, 2, 0), kprobe(20, 9, 2))
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 200, 20, 1), kprobe(20, 9, 2)}, stats.Programs)

	// the classifiers are unloaded and the kprobe is reloaded
	stats, uniquePrograms = collect(kprobe(21, 3, 1))
	assert.Equal(t, []model.EBPFProgramStats{kprobe(21, 12, 3)}, stats.Programs)
	assert.Equal(t, map[programKey]ebpf.ProgramID{kprobeKey: 21}, uniquePrograms)

	// the classifiers are loaded again, their previous total is forgotten
	stats, _ = collect(classifier(10, 0, 0, 0), classifier(14, 5, 1, 0), kprobe(21, 3, 1))
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 5, 1, 0), kprobe(21, 12, 3)}, stats.Programs)

	// ID 14 is reused by a program with lower counters
	stats, _ = collect(classifier(10, 0, 0, 0), classifier(14, 2, 1, 0), kprobe(21, 3, 1))
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 7, 2, 0), kprobe(21, 12, 3)}, stats.Programs)
}

func TestProgramAggregatorUnreadableProgram(t *testing.T) {
	classifier := func(id uint32, runtime time.Duration, runCount uint64) model.EBPFProgramStats {
		return model.EBPFProgramStats{ID: id, Name: "classifier_egress", Type: "SchedCLS", Module: "cws", XlatedProgLen: id, Runtime: runtime, RunCount: runCount}
	}

	agg := newProgramAggregator()
	collect := func(unreadable []ebpf.ProgramID, programs ...model.EBPFProgramStats) []model.EBPFProgramStats {
		var stats model.EBPFStats
		uniquePrograms := make(map[programKey]ebpf.ProgramID)
		for _, id := range unreadable {
			agg.keep(id)
		}
		for i := range programs {
			agg.add(&stats, uniquePrograms, &programs[i])
		}
		agg.flush(&stats, uniquePrograms)
		return stats.Programs
	}

	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 150, 15)}, collect(nil, classifier(10, 100, 10), classifier(11, 50, 5)))

	// clone 11 can't be read once, while clone 10 keeps the entry alive
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 160, 16)}, collect([]ebpf.ProgramID{11}, classifier(10, 110, 11)))

	// once readable again, clone 11 only adds what it ran since it was last read
	assert.Equal(t, []model.EBPFProgramStats{classifier(10, 190, 19)}, collect(nil, classifier(10, 120, 12), classifier(11, 70, 7)))
}
