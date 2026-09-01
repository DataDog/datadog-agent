// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type metricsAttributesAsTagsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/metrics_attributes_as_tags.yml
var metricsAttributesAsTagsConfig string

// TestOTelAgentMetricsAttributesAsTags validates that the infraattributes
// processor's metrics_attributes_as_tags option promotes custom tagger-derived
// tags (e.g. from kubernetesResourcesLabelsAsTags) so they survive the metrics
// translator's allowlist and are emitted as real Datadog metric tags, for DDOT
// (standalone OTel collector).
func TestOTelAgentMetricsAttributesAsTags(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
`
	t.Parallel()
	e2e.Run(t, &metricsAttributesAsTagsTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(metricsAttributesAsTagsConfig),
				),
			),
		)),
	)
}

func (s *metricsAttributesAsTagsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *metricsAttributesAsTagsTestSuite) TestOTLPMetrics() {
	utils.TestMetricsAttributesAsTags(s)
}
