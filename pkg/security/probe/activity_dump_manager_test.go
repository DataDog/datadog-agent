// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

func compareListOfDumps(t *testing.T, out, expectedOut []*ActivityDump) {
	for _, elem := range out {
		var found bool
		for _, expected := range expectedOut {
			if elem.Name == expected.Name {
				found = true
			}
		}

		assert.Equalf(t, true, found, "output didn't match: dump %s should not be in the output", elem.Name)
	}
	for _, elem := range expectedOut {
		var found bool
		for _, got := range out {
			if elem.Name == got.Name {
				found = true
			}
		}

		assert.Equalf(t, true, found, "output didn't match: dump %s is missing from the output", elem.Name)
	}
}

func TestActivityDumpManager_getExpiredDumps(t *testing.T) {
	type fields struct {
		activeDumps []*ActivityDump
	}
	tests := []struct {
		name         string
		fields       fields
		expiredDumps []*ActivityDump
		activeDumps  []*ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*ActivityDump{},
			[]*ActivityDump{},
		},
		{
			"one_dump/one_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{},
		},
		{
			"one_dump/no_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/no_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/5_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*ActivityDump{
					{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(-time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "2", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adm := &ActivityDumpManager{
				activeDumps:        tt.fields.activeDumps,
				ignoreFromSnapshot: make(map[string]bool),
			}

			compareListOfDumps(t, adm.getExpiredDumps(), tt.expiredDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}

func TestActivityDumpManager_getOverweightDumps(t *testing.T) {
	type fields struct {
		activeDumps []*ActivityDump
	}
	tests := []struct {
		name            string
		fields          fields
		overweightDumps []*ActivityDump
		activeDumps     []*ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*ActivityDump{},
			[]*ActivityDump{},
		},
		{
			"one_dump/one_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
			},
			[]*ActivityDump{},
		},
		{
			"one_dump/no_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}},
			},
		},
		{
			"5_dumps/no_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "2"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "3"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "4"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "5"}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}},
				{DumpMetadata: DumpMetadata{Name: "2"}},
				{DumpMetadata: DumpMetadata{Name: "3"}},
				{DumpMetadata: DumpMetadata{Name: "4"}},
				{DumpMetadata: DumpMetadata{Name: "5"}},
			},
		},
		{
			"5_dumps/5_overweight_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "3"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "5"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
				{DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
				{DumpMetadata: DumpMetadata{Name: "3"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
				{DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
				{DumpMetadata: DumpMetadata{Name: "5"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
			},
			[]*ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "3"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "5"}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
				{DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}},
				{DumpMetadata: DumpMetadata{Name: "3"}},
				{DumpMetadata: DumpMetadata{Name: "5"}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "3"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "4"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "5"}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
				{DumpMetadata: DumpMetadata{Name: "2"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "4", End: time.Now().Add(time.Minute)}},
				{DumpMetadata: DumpMetadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "1"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "2"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "3"}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
					{Mutex: &sync.Mutex{}, DumpMetadata: DumpMetadata{Name: "5"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
				},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "4"}, nodeStats: ActivityDumpNodeStats{processNodes: 3}},
				{DumpMetadata: DumpMetadata{Name: "5"}, nodeStats: ActivityDumpNodeStats{processNodes: 2}},
			},
			[]*ActivityDump{
				{DumpMetadata: DumpMetadata{Name: "1"}},
				{DumpMetadata: DumpMetadata{Name: "2"}},
				{DumpMetadata: DumpMetadata{Name: "3"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adm := &ActivityDumpManager{
				activeDumps: tt.fields.activeDumps,
				config: &config.Config{
					ActivityDumpMaxDumpSize: func() int {
						return 2048
					},
				},
				statsdClient:       &statsd.NoOpClient{},
				ignoreFromSnapshot: make(map[string]bool),
			}

			compareListOfDumps(t, adm.getOverweightDumps(), tt.overweightDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}
