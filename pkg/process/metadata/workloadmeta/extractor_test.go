// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

const (
	Pid1 = 1000
	Pid2 = 1001
	Pid3 = 1002
)

func testProc(pid int32, cmdline []string) *procutil.Process {
	return &procutil.Process{
		Pid:     pid,
		Cmdline: cmdline,
		Stats:   &procutil.Stats{CreateTime: time.Now().Unix()},
	}
}

func TestExtractor(t *testing.T) {
	extractor := NewWorkloadMetaExtractor()
	mockGrpcListener := new(mockGrpcListener)
	extractor.grpcListener = mockGrpcListener

	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
		proc2 = testProc(Pid2, []string{"python", "myprogram.py"})
		proc3 = testProc(Pid3, []string{"corrina", "--at-her-best"})
	)

	// Assert that we write all procs on first run
	writeEvents := mockGrpcListener.On("writeEvents", []*ProcessEntity{}, []*ProcessEntity{
		{
			pid:      proc1.Pid,
			language: &languagemodels.Language{Name: languagemodels.Java},
		},
		{
			pid:      proc2.Pid,
			language: &languagemodels.Language{Name: languagemodels.Python},
		},
	})
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid1, proc1.Stats.CreateTime): {
			pid:      proc1.Pid,
			language: &languagemodels.Language{Name: languagemodels.Java},
		},
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			pid:      proc2.Pid,
			language: &languagemodels.Language{Name: languagemodels.Python},
		},
	}, extractor.cache)
	mockGrpcListener.AssertExpectations(t)
	writeEvents.Unset()

	// Assert that we write no duplicates
	writeEvents = mockGrpcListener.On("writeEvents", []*ProcessEntity{}, []*ProcessEntity{})
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid1, proc1.Stats.CreateTime): {
			pid:      proc1.Pid,
			language: &languagemodels.Language{Name: languagemodels.Java},
		},
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			pid:      proc2.Pid,
			language: &languagemodels.Language{Name: languagemodels.Python},
		},
	}, extractor.cache)
	mockGrpcListener.AssertExpectations(t)
	writeEvents.Unset()

	// Assert that old events are evicted from the cache
	writeEvents = mockGrpcListener.On("writeEvents", []*ProcessEntity{
		{
			pid:      Pid1,
			language: &languagemodels.Language{Name: languagemodels.Java},
		},
	}, []*ProcessEntity{
		{
			pid:      Pid3,
			language: &languagemodels.Language{Name: languagemodels.Unknown},
		},
	})
	extractor.Extract(map[int32]*procutil.Process{
		Pid2: proc2,
		Pid3: proc3,
	})
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			pid:      proc2.Pid,
			language: &languagemodels.Language{Name: languagemodels.Python},
		},
		hashProcess(Pid3, proc3.Stats.CreateTime): {
			pid:      proc3.Pid,
			language: &languagemodels.Language{Name: languagemodels.Unknown},
		},
	}, extractor.cache)
	mockGrpcListener.AssertExpectations(t)
	writeEvents.Unset()
}

var _ mockableGrpcListener = (*mockGrpcListener)(nil)

type mockGrpcListener struct {
	mock.Mock
}

func (m *mockGrpcListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {
	// Sometimes the arguments come out of order. This is okay. Sort them so we can assert on their values.
	sort.SliceStable(procsToDelete, func(i, j int) bool {
		return procsToDelete[i].pid < procsToDelete[j].pid
	})
	sort.SliceStable(procsToAdd, func(i, j int) bool {
		return procsToAdd[i].pid < procsToAdd[j].pid
	})

	m.Called(procsToDelete, procsToAdd)
}

func BenchmarkHashProcess(b *testing.B) {
	b.Run("itoa", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hashProcess(0, 0)
		}
	})
	b.Run("sprintf", func(b *testing.B) {
		hashProcess := func(pid int32, createTime int64) string {
			return fmt.Sprintf("pid:%v|createTime:%v", pid, createTime)
		}

		for i := 0; i < b.N; i++ {
			hashProcess(0, 0)
		}
	})
}
