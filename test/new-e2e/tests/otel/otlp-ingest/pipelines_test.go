// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e OTLP Ingest tests
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
    metrics:
      resource_attributes_as_tags: true
`
	t.Parallel()
	e2e.Run(t, &otlpIngestTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestTestSuite) TestOTLPTraces() {
	utils.TestTraces(s)
}

func (s *otlpIngestTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s)
}

func (s *otlpIngestTestSuite) TestOTLPLogs() {
	utils.TestLogs(s)
}

func (s *otlpIngestTestSuite) TestHosts() {
	utils.TestHosts(s)
}
