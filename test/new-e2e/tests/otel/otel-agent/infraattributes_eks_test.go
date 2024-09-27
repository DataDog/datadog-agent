// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type iaEKSTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTelAgentIAEKS(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &iaEKSTestSuite{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(awskubernetes.WithEKSLinuxNodeGroup(), awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(iaConfig)))))
}

func (s *iaEKSTestSuite) TestCalendarJavaApp() {
	utils.TestCalendarJavaApp(s)
}

func (s *iaEKSTestSuite) TestCalendarGoApp() {
	utils.TestCalendarGoApp(s)
}

func (s *iaEKSTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, true)
}

func (s *iaEKSTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, true)
}

func (s *iaEKSTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, true)
}

func (s *iaEKSTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *iaEKSTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}
