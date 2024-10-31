// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpingest contains e2e OTLP Ingest tests
package otlpingest

import (
	_ "embed"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestSamplingTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestSampling(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
agents:
  containers:
    agent:
      env:
        - name: DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS
          value: 'true'
        - name: DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE
          value: '50'
    traceAgent:
      env:
        - name: DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE
          value: '50'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestSamplingTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestSamplingTestSuite) TestSampling() {
	utils.TestSampling(s, false)
}
