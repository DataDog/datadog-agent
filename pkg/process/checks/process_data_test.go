// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
)

type mockExtractor struct {
	called int
	procs  map[int32]*procutil.Process
}

func (e *mockExtractor) Extract(p map[int32]*procutil.Process) {
	e.called += 1
	e.procs = p
}

func newMockExtractor() *mockExtractor {
	return &mockExtractor{}
}

func TestProcessDataFetch(t *testing.T) {
	probe := mocks.NewProbe(t)

	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	processesByPid := map[int32]*procutil.Process{
		proc1.Pid: proc1,
		proc2.Pid: proc2,
	}

	tests := []struct {
		name       string
		extractors []*mockExtractor
		wantErr    bool
	}{
		{
			name:       "error fetching process data",
			extractors: nil,
			wantErr:    true,
		},
		{
			name:       "verify fetch triggers extractors",
			extractors: []*mockExtractor{newMockExtractor(), newMockExtractor()},
			wantErr:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProcessData()
			p.probe = probe

			for _, e := range tc.extractors {
				p.Register(e)
			}

			if tc.wantErr {
				probe.On("ProcessesByPID", mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("unable to retrieve process data"))
				assert.Error(t, p.Fetch())
			} else {
				probe.On("ProcessesByPID", mock.Anything, mock.Anything).
					Return(processesByPid, nil)
				assert.NoError(t, p.Fetch())
				for _, e := range tc.extractors {
					assert.Equal(t, 1, e.called)
					assert.Equal(t, processesByPid, e.procs)
				}
			}
		})
	}
}
