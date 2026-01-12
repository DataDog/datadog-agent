// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type iaTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/ia.yml
var iaConfig string

func TestOTelAgentIA(t *testing.T) {
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
	e2e.Run(t, &iaTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(iaConfig),
				),
			),
		)),
	)
}

var iaParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.HighCardinality,
}

func (s *iaTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *iaTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, iaParams)
}

func (s *iaTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, iaParams)
}

func (s *iaTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, iaParams)
}

func (s *iaTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *iaTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}
