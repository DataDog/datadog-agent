// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
)

const (
	incorrectAssertion = "INCORRECT_ASSERTION"
	invalidTest        = "INVALID_TEST"
)

func runAssertions(cfg common.SyntheticsTestConfig, result common.NetStats) []common.AssertionResult {
	assertions := make([]common.AssertionResult, 0)
	for _, assertion := range cfg.Config.Assertions {
		assertions = append(assertions, runAssertion(assertion, result))
	}
	return assertions
}

func runAssertion(assertion common.Assertion, stats common.NetStats) common.AssertionResult {
	var actual float64
	invalidAssertion := common.AssertionResult{
		Operator: assertion.Operator,
		Type:     assertion.Type,
		Property: assertion.Property,
		Expected: assertion.Target,
		Valid:    false,
	}

	if (stats.Latency == nil || stats.Latency.Max == 0 || stats.PacketsReceived == 0) && assertion.Type == common.AssertionTypeLatency || (stats.Jitter == nil || stats.PacketsReceived < 2 || stats.Latency.Max == 0) && assertion.Type == common.AssertionTypePacketJitter {
		return invalidAssertion
	}

	switch assertion.Type {
	case common.AssertionTypePacketLoss:
		actual = float64(stats.PacketLossPercentage)
	case common.AssertionTypePacketJitter:
		actual = *stats.Jitter
	case common.AssertionTypeLatency:
		switch *assertion.Property {
		case common.AssertionSubTypeAverage:
			actual = stats.Latency.Avg
		case common.AssertionSubTypeMin:
			actual = stats.Latency.Min
		case common.AssertionSubTypeMax:
			actual = stats.Latency.Max
		default:
			return invalidAssertion
		}
	case common.AssertionTypeNetworkHops:
		switch *assertion.Property {
		case common.AssertionSubTypeAverage:
			actual = stats.Hops.Avg
		case common.AssertionSubTypeMin:
			actual = float64(stats.Hops.Min)
		case common.AssertionSubTypeMax:
			actual = float64(stats.Hops.Max)
		default:
			return invalidAssertion
		}
	default:
		return invalidAssertion
	}

	assertionResult := common.AssertionResult{
		Operator: assertion.Operator,
		Type:     assertion.Type,
		Property: assertion.Property,
		Expected: assertion.Target,
		Actual:   actual,
	}
	if err := assertionResult.Compare(); err != nil {
		assertionResult.Valid = false
		return assertionResult
	}
	return assertionResult
}
