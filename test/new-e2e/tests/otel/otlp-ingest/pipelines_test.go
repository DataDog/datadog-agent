// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpingest contains e2e OTLP Ingest tests
package otlpingest

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

type otlpIngestTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngest(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    traceAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'disable_operation_and_resource_name_logic_v2'
    agent:
      env:
        - name: DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS
          value: 'true'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(values))))),
	)
}

var otlpIngestParams = utils.IAParams{
	InfraAttributes: true,
}

func (s *otlpIngestTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, otlpIngestParams)
}

func (s *otlpIngestTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, otlpIngestParams)
}

func (s *otlpIngestTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, otlpIngestParams)
}
