// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type gatewayTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/gateway.yml
var gatewayConfig string

func TestOTelAgentGateway(t *testing.T) {
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
        - name: DD_APM_FEATURES
          value: 'disable_operation_and_resource_name_logic_v2'
`
	t.Parallel()
	e2e.Run(t, &gatewayTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelGatewayConfig(gatewayConfig),
					kubernetesagentparams.WithOTelAgentGateway(),
				),
			),
		)),
	)
}

var gatewayParams = utils.IAParams{
	InfraAttributes: false,
	EKS:             false,
	Cardinality:     types.LowCardinality,
}

func (s *gatewayTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *gatewayTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, gatewayParams)

	// In gateway mode, the agent hostname should be empty (not set)
	// because the gateway agent acts as a forwarder and doesn't represent a specific host.
	traces, err := s.Env().FakeIntake.Client().GetTraces()
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), traces)
	for _, trace := range traces {
		assert.Empty(s.T(), trace.HostName, "agent hostname should be empty in gateway mode")
	}
}

func (s *gatewayTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, gatewayParams)
}

func (s *gatewayTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, gatewayParams)
}

func (s *gatewayTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *gatewayTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *gatewayTestSuite) TestOTelGatewayInstalled() {
	utils.TestOTelGatewayInstalled(s)
}

func (s *gatewayTestSuite) TestOTelGatewayFlare() {
	utils.TestOTelGatewayFlareCmd(s)
}
