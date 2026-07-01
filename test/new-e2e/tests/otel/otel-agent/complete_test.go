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

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type completeTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/complete.yml
var completeConfig string

func TestOTelAgentComplete(t *testing.T) {
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
        - name: DD_OTELCOLLECTOR_CONVERTER_ENABLED
          value: 'false'
        - name: DD_APM_FEATURES
          value: 'disable_receive_resource_spans_v2,disable_operation_and_resource_name_logic_v2'
`
	t.Parallel()
	e2e.Run(t, &completeTestSuite{},
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenkindvm.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(values),
						kubernetesagentparams.WithOTelAgent(),
						kubernetesagentparams.WithOTelConfig(completeConfig),
					),
				),
			),
		),
	)
}

func (s *completeTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

// completeParams mirrors minimalParams but flags the v1 OTLP receiver path
// (disable_receive_resource_spans_v2 is set in the suite's agent config), which
// still emits otel.library.name/version instead of otel.scope.name/version.
var completeParams = utils.IAParams{
	InfraAttributes:        minimalParams.InfraAttributes,
	EKS:                    minimalParams.EKS,
	Cardinality:            minimalParams.Cardinality,
	ReceiveResourceSpansV1: true,
}

func (s *completeTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, completeParams)
}

func (s *completeTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, completeParams)
}

func (s *completeTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, completeParams)
}

func (s *completeTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *completeTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}

func (s *completeTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}
