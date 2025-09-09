// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func (s *SyntheticsTestScheduler) runAssertions(cfg common.SyntheticsTestConfig, result payload.NetworkPath) ([]common.AssertionResult, error) {
	assertions := make([]common.AssertionResult, 0)
	for _, assertion := range cfg.Config.Assertions {
		assertionResult, err := s.runAssertion(assertion, result)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, assertionResult)
	}
	return assertions, nil
}

func (s *SyntheticsTestScheduler) runAssertion(assertion common.Assertion, stats common.NetStats) (common.AssertionResult, error) {
	var actual interface{}

	switch assertion.Field {
	case "packetLossPercentage":
		actual = stats.PacketLossPercentage
	case "jitter":
		actual = stats.Jitter
	case "latency.avg":
		actual = stats.Latency.Avg
	case "latency.min":
		actual = stats.Latency.Min
	case "latency.max":
		actual = stats.Latency.Max
	case "hops.avg":
		actual = stats.Hops.Avg
	case "hops.min":
		actual = stats.Hops.Min
	case "hops.max":
		actual = stats.Hops.Max
	default:
		return common.AssertionResult{}, fmt.Errorf("unsupported field: %s", assertion.Field)
	}

	assertionResult := common.AssertionResult{
		Operator: assertion.Operator,
		Type:     assertion.Field,
		Expected: assertion.Expected,
		Actual:   actual,
	}
	if err := assertionResult.Compare(); err != nil {
		return assertionResult, err
	}
	return assertionResult, nil
}
