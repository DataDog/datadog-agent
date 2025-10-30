// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package checks

import (
	"testing"
	"time"

	probemocks "github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestProcessByPID ensures the usage of the probe when wlm collection is ON/OFF
func TestProcessByPID(t *testing.T) {
	for _, tc := range []struct {
		description      string
		useWLMCollection bool
	}{
		{
			description:      "wlm collection ENABLED",
			useWLMCollection: true,
		},
		{
			description:      "wlm collection DISABLED",
			useWLMCollection: false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			// INITIALIZATION
			mockProbe := probemocks.NewProbe(t)
			mockConstantClock := constantMockClock(time.Now())
			processCheck := &ProcessCheck{
				probe: mockProbe,
				clock: mockConstantClock,
			}

			// MOCKING
			// wlm ListProcesses should also not be called, but we cannot test that with the current mock implementation
			mockProbe.AssertNotCalled(t, "StatsForPIDs")
			mockProbe.EXPECT().ProcessesByPID(mockConstantClock.Now(), mock.Anything).Return(nil, nil).Once()
			// TESTING
			// collectStats is irrelevant since it should not impact which functions are called
			_, err := processCheck.processesByPID()
			assert.NoError(t, err)
		})
	}
}
