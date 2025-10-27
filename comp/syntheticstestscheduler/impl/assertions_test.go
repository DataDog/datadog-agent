// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func TestRunAssertion(t *testing.T) {
	tests := []struct {
		name      string
		assertion common.Assertion
		stats     common.NetStats
		wantValue string
		valid     bool
	}{
		{
			name: "PacketLoss assertion",
			assertion: common.Assertion{
				Type:     common.AssertionTypePacketLoss,
				Operator: common.OperatorLessThan,
				Target:   "50",
			},
			stats:     common.NetStats{PacketLossPercentage: 42},
			wantValue: "42",
			valid:     true,
		},
		{
			name: "Jitter assertion",
			assertion: common.Assertion{
				Type:     common.AssertionTypePacketJitter,
				Operator: common.OperatorLessThan,
				Target:   "5",
			},
			stats:     common.NetStats{Jitter: 3.5},
			valid:     true,
			wantValue: "3.5",
		},
		{
			name: "Latency avg assertion",
			assertion: common.Assertion{
				Type:     common.AssertionTypeLatency,
				Property: common.AssertionSubTypeAverage,
				Operator: common.OperatorLessThan,
				Target:   "100",
			},
			stats:     common.NetStats{Latency: payload.E2eProbeRttLatency{Avg: 90, Min: 70, Max: 120}},
			valid:     true,
			wantValue: "90",
		},
		{
			name: "Latency unsupported property",
			assertion: common.Assertion{
				Type:     common.AssertionTypeLatency,
				Property: "median",
				Operator: common.OperatorLessThan,
				Target:   "100",
			},
			stats: common.NetStats{},
			valid: false,
		},
		{
			name: "Hops max assertion",
			assertion: common.Assertion{
				Type:     common.AssertionTypeNetworkHops,
				Property: common.AssertionSubTypeMax,
				Operator: common.OperatorLessThan,
				Target:   "15",
			},
			stats:     common.NetStats{Hops: payload.HopCountStats{Max: 12}},
			valid:     true,
			wantValue: "12",
		},
		{
			name: "Unsupported assertion type",
			assertion: common.Assertion{
				Type:     "UNKNOWN_TYPE",
				Operator: common.OperatorLessThan,
				Target:   "1",
			},
			stats: common.NetStats{},
			valid: false,
		},
		{
			name: "Comparison failure",
			assertion: common.Assertion{
				Type:     common.AssertionTypePacketLoss,
				Operator: common.OperatorLessThan,
				Target:   "10",
			},
			stats:     common.NetStats{PacketLossPercentage: 50},
			wantValue: "50",
			valid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runAssertion(tt.assertion, tt.stats)
			if got.Valid != tt.valid {
				t.Errorf("expected valid=%v, got=%v", tt.valid, got.Valid)
			}
		})
	}
}

func TestRunAssertions(t *testing.T) {
	tests := []struct {
		name         string
		cfg          common.SyntheticsTestConfig
		stats        common.NetStats
		wantLength   int
		wantValid    int
		wantNotValid int
	}{
		{
			name: "Single assertion success",
			cfg: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Assertions: []common.Assertion{
						{
							Type:     common.AssertionTypePacketLoss,
							Operator: common.OperatorLessThan,
							Target:   "60",
						},
					},
				},
			},
			stats:        common.NetStats{PacketLossPercentage: 42},
			wantLength:   1,
			wantValid:    1,
			wantNotValid: 0,
		},
		{
			name: "Multiple assertions with mixed results",
			cfg: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Assertions: []common.Assertion{
						{
							Type:     common.AssertionTypePacketLoss,
							Operator: common.OperatorLessThan,
							Target:   "10", // should fail
						},
						{
							Type:     common.AssertionTypePacketJitter,
							Operator: common.OperatorLessThan,
							Target:   "10", // should pass
						},
						{
							Type:     "UNKNOWN_TYPE",
							Operator: common.OperatorIs,
							Target:   "1", // unsupported type → failure
						},
					},
				},
			},
			stats: common.NetStats{
				PacketLossPercentage: 42,
				Jitter:               3.5,
			},
			wantLength:   3,
			wantValid:    1,
			wantNotValid: 2,
		},
		{
			name: "No assertions",
			cfg: common.SyntheticsTestConfig{
				Config: struct {
					Assertions []common.Assertion   `json:"assertions"`
					Request    common.ConfigRequest `json:"request"`
				}{
					Assertions: []common.Assertion{},
				},
			},
			stats:        common.NetStats{},
			wantLength:   0,
			wantValid:    0,
			wantNotValid: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := runAssertions(tt.cfg, tt.stats)

			if len(results) != tt.wantLength {
				t.Errorf("expected %d results, got %d", tt.wantLength, len(results))
			}

			errors := 0
			for _, r := range results {
				if !r.Valid {
					errors++
				}
			}

			if errors != tt.wantNotValid {
				t.Errorf("expected %d failures, got %d", tt.wantNotValid, errors)
			}
		})
	}
}
