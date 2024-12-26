// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otelAgentSpanReceiverV2TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentSpanReceiverV2(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    otelAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'enable_receive_resource_spans_v2'
        - name: DD_OTLP_CONFIG_TRACES_SPAN_NAME_AS_RESOURCE_NAME
          value: 'false'
`
	t.Parallel()
	e2e.Run(t, &otelAgentSpanReceiverV2TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(minimalConfig)))))
}

func (s *otelAgentSpanReceiverV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otelAgentSpanReceiverV2TestSuite) TestTracesWithSpanReceiverV2() {
	utils.TestTracesWithSpanReceiverV2(s)
}
