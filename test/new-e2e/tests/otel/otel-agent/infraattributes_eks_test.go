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
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type iaEKSTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentIAEKS(t *testing.T) {
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
	e2e.Run(t, &iaEKSTestSuite{},
		e2e.WithProvisioner(
			proveks.Provisioner(
				proveks.WithRunOptions(
					eks.WithEKSOptions(
						eks.WithLinuxNodeGroup(),
					),
					eks.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(values),
						kubernetesagentparams.WithOTelAgent(),
						kubernetesagentparams.WithOTelConfig(iaConfig),
					),
				))),
	)
}

var eksParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             true,
	Cardinality:     types.HighCardinality,
}

func (s *iaEKSTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *iaEKSTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, eksParams)
}

func (s *iaEKSTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, eksParams)
}

func (s *iaEKSTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, eksParams)
}

func (s *iaEKSTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *iaEKSTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}

type iaUSTEKSTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentIAUSTEKS(t *testing.T) {
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
	e2e.Run(t, &iaUSTEKSTestSuite{}, e2e.WithProvisioner(
		proveks.Provisioner(
			proveks.WithRunOptions(
				eks.WithEKSOptions(
					eks.WithLinuxNodeGroup(),
				),
				eks.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(iaConfig),
				),
			))),
	)
}

func (s *iaUSTEKSTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, true, utils.CalendarService)
}

func (s *iaUSTEKSTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, eksParams)
}

func (s *iaUSTEKSTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, eksParams)
}

func (s *iaUSTEKSTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, eksParams)
}
