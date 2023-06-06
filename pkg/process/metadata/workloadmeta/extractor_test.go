// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/languagedetection"
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
			cmdline:  proc1.Cmdline,
			language: &languagedetection.Language{Name: languagedetection.Java},
		},
		{
			pid:      proc2.Pid,
			cmdline:  proc2.Cmdline,
			language: &languagedetection.Language{Name: languagedetection.Python},
		},
	})
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})
	mockGrpcListener.AssertExpectations(t)
	writeEvents.Unset()

	// Assert that we write no duplicates
	writeEvents = mockGrpcListener.On("writeEvents", []*ProcessEntity{}, []*ProcessEntity{})
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})
	mockGrpcListener.AssertExpectations(t)
	writeEvents.Unset()

	// Assert that old events are evicted from the cache
	writeEvents = mockGrpcListener.On("writeEvents", []*ProcessEntity{
		{
			pid:      Pid1,
			cmdline:  proc1.Cmdline,
			language: &languagedetection.Language{Name: languagedetection.Java},
		},
	}, []*ProcessEntity{
		{
			pid:      Pid3,
			cmdline:  proc3.Cmdline,
			language: &languagedetection.Language{Name: languagedetection.Unknown},
		},
	})
	extractor.Extract(map[int32]*procutil.Process{
		Pid2: proc2,
		Pid3: proc3,
	})
	mockGrpcListener.AssertExpectations(t)

}

var _ mockableGrpcListener = (*mockGrpcListener)(nil)

type mockGrpcListener struct {
	mock.Mock
}

func (m *mockGrpcListener) writeEvents(procsToDelete, procsToAdd []*ProcessEntity) {
	m.Called(procsToDelete, procsToAdd)
}
