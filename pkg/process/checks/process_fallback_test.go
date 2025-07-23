// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package checks

import (
	"testing"
	"time"

	wmimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	probemocks "github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/stretchr/testify/assert"
)

// TestProcessByPID ensures the usage of the probe when wlm collection is ON/OFF
func TestProcessByPID(t *testing.T) {
	for _, tc := range []struct {
		description      string
		useWLMCollection bool
		collectStats     bool
	}{
		{
			description:      "wlm collection ENABLED, with stats ENABLED",
			useWLMCollection: true,
			collectStats:     true,
		},
		{
			description:      "wlm collection ENABLED, with stats DISABLED",
			useWLMCollection: true,
			collectStats:     false,
		},
		{
			description:      "wlm collection DISABLED, with stats ENABLED",
			useWLMCollection: false,
			collectStats:     true,
		},
		{
			description:      "wlm collection DISABLED, with stats DISABLED",
			useWLMCollection: false,
			collectStats:     false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			// INITIALIZATION
			mockProbe := probemocks.NewProbe(t)
			mockWLM := wmimpl.NewMockWLM(t)
			mockConstantClock := constantMockClock(time.Now())
			processCheck := &ProcessCheck{
				wmeta:                   mockWLM,
				probe:                   mockProbe,
				useWLMProcessCollection: tc.useWLMCollection,
				clock:                   mockConstantClock,
			}

			// MOCKING
			mockWLM.AssertNotCalled(t, "ListProcesses")
			mockProbe.AssertNotCalled(t, "StatsForPIDs")
			mockProbe.EXPECT().ProcessesByPID(mockConstantClock.Now(), tc.collectStats).Return(nil, nil).Once()
			// TESTING
			_, err := processCheck.processesByPID(tc.collectStats)
			assert.NoError(t, err)
		})
	}
}
