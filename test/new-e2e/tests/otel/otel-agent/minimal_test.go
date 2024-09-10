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

type minimalTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/minimal.yml
var minimalConfig string

func TestOTelAgentMinimal(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &minimalTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(minimalConfig)))))
}

func (s *minimalTestSuite) TestOTLPTraces() {
	utils.TestTraces(s)
}

func (s *minimalTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s)
}

func (s *minimalTestSuite) TestOTLPLogs() {
	utils.TestLogs(s)
}

func (s *minimalTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *minimalTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *minimalTestSuite) TestOTelFlare() {
	utils.TestOTelFlare(s)
}
