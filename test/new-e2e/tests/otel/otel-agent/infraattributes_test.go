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

type iaTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/ia.yml
var iaConfig string

func TestOTelAgentIA(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &iaTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(iaConfig)))))
}

func (s *iaTestSuite) TestCalendarJavaApp() {
	utils.TestCalendarJavaApp(s)
}

func (s *iaTestSuite) TestCalendarGoApp() {
	utils.TestCalendarGoApp(s)
}

func (s *iaTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, true)
}

func (s *iaTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, true)
}

func (s *iaTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, true)
}

func (s *iaTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *iaTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}
