// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package thermal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/pdhutil"
)

// toCelsius converts Kelvin to Celsius using runtime float64 arithmetic,
// matching the production code behavior (avoids compile-time constant folding).
func toCelsius(kelvin float64) float64 {
	return kelvin - kelvinOffset
}

func TestThermalCheck(t *testing.T) {
	type zone struct {
		instance  string
		tempK     float64
		hpTenthsK float64 // 0 = High Precision Temperature counter unavailable
		passive   float64
	}
	tests := []struct {
		name  string
		zones []zone
	}{
		{
			name: "prefers High Precision Temperature when available",
			zones: []zone{
				{instance: "tz.thm0", tempK: 345.0, hpTenthsK: 3452.0, passive: 100.0},
			},
		},
		{
			name: "falls back to Temperature when High Precision is unavailable",
			zones: []zone{
				{instance: "tz.thm0", tempK: 345.0, passive: 100.0},
			},
		},
		{
			name: "emits metrics for each zone when multiple zones exist",
			zones: []zone{
				{instance: "tz.thm0", tempK: 345.0, hpTenthsK: 3450.0, passive: 100.0},
				{instance: "tz.thm1", tempK: 360.0, hpTenthsK: 3600.0, passive: 80.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdhtest.SetupTesting(`..\testfiles\counter_indexes_en-us.txt`, `..\testfiles\allcounters_en-us.txt`)
			check := new(thermalCheck)
			mock := mocksender.NewMockSender(check.ID())
			require.NoError(t, check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider"))

			for _, z := range tt.zones {
				pdhtest.AddCounterInstance("Thermal Zone Information", z.instance)
				path := func(counter string) string {
					return fmt.Sprintf(`\\.\Thermal Zone Information(%s)\%s`, z.instance, counter)
				}
				pdhtest.SetQueryReturnValue(path("Temperature"), z.tempK)
				if z.hpTenthsK > 0 {
					pdhtest.SetQueryReturnValue(path("High Precision Temperature"), z.hpTenthsK)
				}
				pdhtest.SetQueryReturnValue(path("% Passive Limit"), z.passive)

				expectedK := z.tempK
				if z.hpTenthsK > 0 {
					expectedK = z.hpTenthsK / 10.0
				}
				tags := []string{"thermal_zone:" + z.instance}
				mock.On("Gauge", "system.thermal.temperature", toCelsius(expectedK), "", tags).Return().Once()
				mock.On("Gauge", "system.thermal.passive_limit", z.passive, "", tags).Return().Once()
			}
			mock.On("Commit").Return().Once()
			require.NoError(t, check.Run())
			mock.AssertExpectations(t)
		})
	}
}

// TestThermalCheckNoInstances verifies that the check handles PDH_NO_DATA (no
// thermal zone instances on the host, e.g. VMs) silently — no error, no
// warning, no metrics, just a Commit.
func TestThermalCheckNoInstances(t *testing.T) {
	pdhtest.SetupTesting(`..\testfiles\counter_indexes_en-us.txt`, `..\testfiles\allcounters_en-us.txt`)
	pdhtest.SetMockCollectQueryDataReturn(pdhtest.PDH_NO_DATA)
	t.Cleanup(func() {
		pdhtest.SetMockCollectQueryDataReturn(0)
	})

	check := new(thermalCheck)
	mock := mocksender.NewMockSender(check.ID())
	require.NoError(t, check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider"))

	mock.On("Commit").Return().Once()
	require.NoError(t, check.Run())

	mock.AssertExpectations(t)
	mock.AssertNotCalled(t, "Gauge")
}

func TestIsNotTotal(t *testing.T) {
	assert.True(t, isNotTotal("tz.thm0"))
	assert.False(t, isNotTotal("_Total"))
	assert.False(t, isNotTotal("_total"))
}
