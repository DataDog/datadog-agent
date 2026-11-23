// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("Configurations", configurations) }

// configurations runs the tests of the check configurations that the other tests do not use: the default collector
// selection, and a cluster that serves older API versions.
func configurations(t *testing.T, s *suite.Suite) {
	s.RunTests(t,
		// Without a collector selection, the check collects the stable resources that discovery reports.
		suite.Test{
			Name:  "DefaultCollectors",
			Setup: suite.Setup{DefaultCollectors: true},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name:   "delete every object",
					Change: suite.DeleteAll(suite.Collected),
					Expect: suite.Deleted(suite.Collected),
				},
			},
		},
		// A cluster that only serves the older versions of the resources that have several, like older operators. The
		// check selects the served version through discovery: a collector that the configuration names uses its default
		// version.
		suite.Test{
			Name:  "OlderVersions",
			Setup: suite.Setup{DefaultCollectors: true, OlderVersions: true},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name:   "delete every object",
					Change: suite.DeleteAll(suite.Collected),
					Expect: suite.Deleted(suite.Collected),
				},
			},
		},
	)
}
