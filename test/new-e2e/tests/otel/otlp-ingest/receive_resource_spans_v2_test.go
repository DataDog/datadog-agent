// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpingest

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestSpanReceiverV2TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestSpanReceiverV2(t *testing.T) {
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
          value: 'enable_receive_resource_spans_v2'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestSpanReceiverV2TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestSpanReceiverV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestSpanReceiverV2TestSuite) TestTracesWithSpanReceiverV2() {
	utils.TestTracesWithSpanReceiverV2(s)
}
