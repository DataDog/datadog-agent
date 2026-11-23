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

func init() { suite.Register("Discovery", discovery) }

// discovery runs the tests of the API discovery of the check.
func discovery(t *testing.T, s *suite.Suite) {
	s.RunTests(t,
		// A check whose API discovery fails when it is configured collects every resource once the discovery
		// recovers.
		suite.Test{
			Name:  "FailureAtStartRecovers",
			Setup: suite.Setup{BeforeConfigure: suite.FailDiscovery(errors.New("the API server is unavailable"))},
			Steps: []suite.Step{
				{
					Name:   "run without discovery",
					Expect: suite.NoObjects,
				},
				{
					Name:   "the discovery recovers",
					Change: suite.FailDiscovery(nil),
					Expect: suite.Live(suite.Collected),
				},
			},
		},
	)
}
