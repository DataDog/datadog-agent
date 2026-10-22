// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpingest contains e2e OTLP Ingest tests
package otlpingest

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestMetricsAttributesAsTagsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestOTLPIngestMetricsAttributesAsTags validates that the infraattributes
// processor's metrics_attributes_as_tags option (exposed as
// otlp_config.metrics.infra_attributes.as_tags) promotes custom tagger-derived
// tags (e.g. from kubernetesResourcesLabelsAsTags) so they survive the metrics
// translator's allowlist and are emitted as real Datadog metric tags, for
// core-agent OTLP ingestion.
func TestOTLPIngestMetricsAttributesAsTags(t *testing.T) {
	// Logs are enabled so TestCalendarApp's log-based startup check (which waits
	// for the calendar app's OTLP logs to reach the intake) passes;
	// containerCollectAll is disabled so only the app's OTLP logs are collected.
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    metrics:
      enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    agent:
      env:
        - name: DD_OTLP_CONFIG_METRICS_INFRA_ATTRIBUTES_AS_TAGS
          value: 'true'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestMetricsAttributesAsTagsTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(values))))),
	)
}

func (s *otlpIngestMetricsAttributesAsTagsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// BaseSuite.SetupSuite already registers a t.Cleanup hook that runs
	// CleanupOnSetupFailure; deferring it here would consume setup panics before
	// testify can report their stack (see suite.go CleanupOnSetupFailure docs).

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestMetricsAttributesAsTagsTestSuite) TestOTLPMetrics() {
	utils.TestMetricsAttributesAsTags(s)
}
