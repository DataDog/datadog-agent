// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestEventStringer(t *testing.T) {
	testCases := []struct {
		ev      event
		wantStr string
	}{
		{
			ev: eventProcessesUpdated{
				updated: []ProcessUpdate{{ProcessID: ProcessID{PID: 1}}},
				removed: []ProcessID{{PID: 2}},
			},
			wantStr: "eventProcessesUpdated{updated: 1, removed: 1}",
		},
		{
			ev: eventProgramLoaded{
				programID: 1,
			},
			wantStr: "eventProgramLoaded{programID: 1}",
		},
		{
			ev: eventProgramLoadingFailed{
				programID: 1,
				err:       errors.New("load error"),
			},
			wantStr: `eventProgramLoadingFailed{programID: 1, err: load error}`,
		},
		{
			ev: eventProgramAttached{
				program: &attachedProgram{
					ir:     &ir.Program{ID: 1},
					procID: ProcessID{PID: 100},
				},
			},
			wantStr: "eventProgramAttached{programID: 1, processID: {PID:100}}",
		},
		{
			ev: eventProgramAttached{
				program: nil,
			},
			wantStr: "eventProgramAttached{program: nil}",
		},
		{
			ev: eventProgramAttachingFailed{
				programID: 1,
				processID: ProcessID{PID: 100},
				err:       errors.New("attach error"),
			},
			wantStr: `eventProgramAttachingFailed{programID: 1, processID: {PID:100}, err: attach error}`,
		},
		{
			ev: eventProgramDetached{
				programID: 1,
				processID: ProcessID{PID: 100},
			},
			wantStr: "eventProgramDetached{programID: 1, processID: {PID:100}}",
		},
		{
			ev:      eventShutdown{},
			wantStr: "eventShutdown{}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.wantStr, func(t *testing.T) {
			assert.Equal(t, tc.wantStr, tc.ev.String())
		})
	}
}
