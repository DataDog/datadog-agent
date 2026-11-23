// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package cases registers the tests of the orchestrator check suite, one group for each area.
package cases

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
)

func init() { suite.Register("Coverage", coverage) }

// coverage checks that the scenarios serve every registry entry, in its first and last versions, and expect the check
// to send it, so that no entry is left out of the suite without a reason in suite.NotListed.
func coverage(t *testing.T, _ *suite.Suite) {
	for _, cfg := range []suite.ScenarioConfig{{AllCollectors: true}, {AllCollectors: true, OlderVersions: true}} {
		t.Run(cfg.String(), func(t *testing.T) {
			objects := suite.ScenarioObjects(t, cfg)
			expected := suite.ExpectedObjects(objects, cfg)
			served := map[string]bool{}
			for _, o := range objects {
				served[o.FullName] = true
				assert.Contains(t, expected.Live, o.UID(), "the scenarios serve %s but do not expect the check to send it", o.FullName)
			}
			for _, resource := range registry.Resources() {
				name := resource.FixtureName(suite.ServedVersion(resource, cfg))
				if _, skipped := suite.NotListed[resource.FixtureName(resource.Versions[0])]; skipped {
					continue
				}
				assert.True(t, served[name], "the scenarios do not serve %s: add it to the fixtures, or to notListed with the reason", name)
			}
		})
	}

	for name := range suite.NotListed {
		found := false
		for _, resource := range registry.Resources() {
			found = found || resource.FixtureName(resource.Versions[0]) == name
		}
		assert.True(t, found, "notListed names %s, which is not a registry entry", name)
	}

}
