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

type iaTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/ia.yml
var iaConfig string

func TestOTelAgentIA(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &iaTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(iaConfig)))))
}

var iaParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.HighCardinality,
}

func (s *iaTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s)
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
