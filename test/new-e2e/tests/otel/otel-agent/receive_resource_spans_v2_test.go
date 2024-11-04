// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"testing"
)

type otlpIngestTestSuiteWithSpanReceiverV2 struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestWithSpanReceiverV2(t *testing.T) {
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
`
	t.Parallel()
	e2e.Run(t, &otlpIngestTestSuiteWithSpanReceiverV2{}, e2e.WithProvisioner(localkubernetes.Provisioner(localkubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(minimalConfig)))))
}

var otlpIngestParams = utils.IAParams{
	InfraAttributes: false,
}

func (s *otlpIngestTestSuiteWithSpanReceiverV2) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestSpanReceiverV2(s)
}

func (s *otlpIngestTestSuiteWithSpanReceiverV2) TestTracesWithSpanReceiverV2() {
	utils.TestTracesWithSpanReceiverV2(s, otlpIngestParams)
}
