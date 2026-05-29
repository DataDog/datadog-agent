// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checksketchsource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// The "anecdote" check name maps to MetricSourceAnecdote in
// pkg/metrics/metricsource.go CheckNameToMetricSource. The corresponding
// OriginCategory in pkg/serializer/internal/metrics/origin_mapping.go is 470.
// It is not a built-in agent check, so no conflict occurs on the test host.
const (
	checkName              = "anecdote"
	distributionMetricName = "e2e.check.sketch.source.distribution"

	// expectedOriginCategory is the OriginCategory written into the sketch
	// protobuf metadata when Source == MetricSourceAnecdote.
	// Value sourced from metricSourceToOriginCategory in origin_mapping.go.
	expectedOriginCategory = uint32(470)
)

// anecdoteCheckPython is a minimal Python check that emits one distribution
// point per run. The class name matches the check name per Agent convention.
const anecdoteCheckPython = `
from datadog_checks.base import AgentCheck

class AnecdoteCheck(AgentCheck):
    def check(self, instance):
        self.distribution("` + distributionMetricName + `", 1.0, tags=[])
`

// anecdoteCheckConf is the minimal conf.yaml for the anecdote check.
const anecdoteCheckConf = `
init_config:
instances:
  - {}
`

type checkSketchSourceSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestCheckSketchSourceSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &checkSketchSourceSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(
						agentparams.WithFile(
							"/etc/datadog-agent/checks.d/anecdote.py",
							anecdoteCheckPython,
							true,
						),
						agentparams.WithIntegration("anecdote.d", anecdoteCheckConf),
					),
				),
			),
		),
		e2e.WithStackName("checksketchsource"),
	)
}

// TestCheckSketchSourcePreservedInOriginMetadata verifies that a distribution
// metric submitted by a check carries a non-zero OriginCategory in the sketch
// payload received by fakeintake.
//
// This is a regression test for the bug in CheckSampler.newSketchSeries where
// ctx.source was never copied to ss.Source, causing all check-submitted
// distributions to arrive with OriginCategory=0 in the protobuf metadata,
// regardless of the check's actual MetricSource.
//
// The "anecdote" check name maps to MetricSourceAnecdote which serialises to
// OriginCategory=470. The test fails before the fix (observes 0) and passes
// after it (observes 470).
func (s *checkSketchSourceSuite) TestCheckSketchSourcePreservedInOriginMetadata() {
	// Wait until at least one sketch for the distribution arrives.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		sketches, err := s.Env().FakeIntake.Client().FilterSketches(distributionMetricName)
		assert.NoError(c, err)
		assert.NotEmpty(c, sketches, "distribution sketch must reach fakeintake")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for distribution sketch to reach fakeintake")

	sketches, err := s.Env().FakeIntake.Client().FilterSketches(distributionMetricName)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), sketches)

	for _, sketch := range sketches {
		require.NotNil(s.T(), sketch.Metadata,
			"sketch metadata must be present in protobuf payload")
		require.NotNil(s.T(), sketch.Metadata.Origin,
			"sketch metadata origin must be present")
		assert.Equal(s.T(), expectedOriginCategory, sketch.Metadata.Origin.OriginCategory,
			"OriginCategory for check %q must be %d (MetricSourceAnecdote); "+
				"got %d — CheckSampler.newSketchSeries is not copying ctx.source to ss.Source",
			checkName, expectedOriginCategory, sketch.Metadata.Origin.OriginCategory,
		)
	}
}
