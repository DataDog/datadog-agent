// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	procinfo "github.com/DataDog/datadog-agent/pkg/dyninst/process"
)

func TestEventStringer(t *testing.T) {
	testCases := []struct {
		ev      event
		wantStr string
	}{
		{
			ev: eventProcessesUpdated{
				updated: []ProcessUpdate{{Info: procinfo.Info{ProcessID: ProcessID{PID: 1}}}},
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
			},
			wantStr: `eventProgramLoadingFailed{programID: 1}`,
		},
		{
			ev: eventProgramAttached{
				program: &attachedProgram{
					loadedProgram: &loadedProgram{
						programID: ir.ProgramID(1),
					},
					processID: ProcessID{PID: 100},
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
			},
			wantStr: `eventProgramAttachingFailed{programID: 1, processID: {PID:100}}`,
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
		{
			ev: eventMissingTypesReported{
				processID: ProcessID{PID: 100},
				typeNames: []string{"Foo", "Bar"},
			},
			wantStr: "eventMissingTypesReported{processID: {PID:100}, typeNames: 2}",
		},
		{
			ev: eventRuntimeStatsUpdated{
				programID: 1,
			},
			wantStr: "eventRuntimeStatsUpdated{programID: 1}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.wantStr, func(t *testing.T) {
			assert.Equal(t, tc.wantStr, tc.ev.String())
		})
	}
}
