// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpingest contains e2e OTLP Ingest tests
package otlpingest

import (
	_ "embed"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestEKSTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestEKS(t *testing.T) {
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
    agent:
      env:
        - name: DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS
          value: 'true'
        - name: DD_TAGS
          value: 'team:infra'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestEKSTestSuite{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(awskubernetes.WithEKSOptions(eks.WithLinuxNodeGroup()), awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)))))
}

var otlpIngestEKSParams = utils.IAParams{
	InfraAttributes: true,
}

func (s *otlpIngestEKSTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s, false)
}

func (s *otlpIngestEKSTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, otlpIngestEKSParams)
}

func (s *otlpIngestEKSTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, otlpIngestEKSParams)
}

func (s *otlpIngestEKSTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, otlpIngestEKSParams)
}
