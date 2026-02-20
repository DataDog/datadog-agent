// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otelAgentSpanReceiverV2TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentSpanReceiverV2(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    otelAgent:
      env:
        - name: DD_OTLP_CONFIG_TRACES_SPAN_NAME_AS_RESOURCE_NAME
          value: 'false'
`
	t.Parallel()
	e2e.Run(t, &otelAgentSpanReceiverV2TestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(
			scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(minimalConfig))),
		)),
	)
}

func (s *otelAgentSpanReceiverV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otelAgentSpanReceiverV2TestSuite) TestTracesWithSpanReceiverV2() {
	utils.TestTracesWithSpanReceiverV2(s)
}
