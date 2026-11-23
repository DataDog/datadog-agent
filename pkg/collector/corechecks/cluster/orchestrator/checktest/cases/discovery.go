// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("Discovery", Discovery) }

// Discovery runs the tests of the API discovery of the check.
func Discovery(t *testing.T, cs *suite.Suite) {
	cs.RunCases(t,
		// A check whose API discovery fails when it is configured collects every resource once the discovery
		// recovers, in every bundle.
		suite.Case{
			Name: "FailureAtStartRecovers",
			Configure: func(cfg *suite.ScenarioConfig) {
				cfg.Setup = func(_ *testing.T, m *suite.ScenarioMocks) {
					m.Discovery.Fail(errors.New("the API server is unavailable"))
				}
			},
			Steps: []suite.Step{
				{Name: "run without discovery", Want: suite.Nothing},
				{
					Name: "the discovery recovers",
					Act:  func(_ *testing.T, sc *suite.Scenario) { sc.Mocks.Discovery.Fail(nil) },
					Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads { return sc.LiveAll(t) },
				},
			},
		},
	)
}
