// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type completeTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/minimal.yml
var completeConfig string

func TestOTelAgentComplete(t *testing.T) {
	values := `
agents:
  containers:
    agent:
      env:
        - name: DD_OTELCOLLECTOR_CONVERTER_ENABLED
          value: false
    otelAgent:
      env:
        - name: DD_OTELCOLLECTOR_CONVERTER_ENABLED
          value: false
`
	t.Parallel()
	e2e.Run(t, &completeTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(completeConfig)))))
}

func (s *completeTestSuite) TestOTLPTraces() {
	utils.TestTraces(s)
}

func (s *completeTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s)
}

func (s *completeTestSuite) TestOTLPLogs() {
	utils.TestLogs(s)
}

func (s *completeTestSuite) TestHosts() {
	utils.TestHosts(s)
}
