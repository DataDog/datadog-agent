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

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type iaEKSTestSuite14 struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentIAEKS(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`

	t.Parallel()
	e2e.Run(t, &iaEKSTestSuite14{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(awskubernetes.WithEKSLinuxNodeGroup(), awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(iaConfig)))))
}

var eksParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             true,
	Cardinality:     types.HighCardinality,
}

func (s *iaEKSTestSuite14) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s)
}

func (s *iaEKSTestSuite14) TestOTLPTraces() {
	utils.TestTraces(s, eksParams)
}

func (s *iaEKSTestSuite14) TestOTLPMetrics() {
	utils.TestMetrics(s, eksParams)
}

func (s *iaEKSTestSuite14) TestOTLPLogs() {
	utils.TestLogs(s, eksParams)
}

func (s *iaEKSTestSuite14) TestHosts() {
	utils.TestHosts(s)
}

func (s *iaEKSTestSuite14) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}
