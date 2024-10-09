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
	localkubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type transformTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/transform.yml
var transformConfig string

func TestOTelAgentTransform(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &transformTestSuite{}, e2e.WithProvisioner(localkubernetes.Provisioner(localkubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(transformConfig)))))
}

func (s *transformTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s)
}

func (s *transformTestSuite) TestTraces() {
	utils.TestTracesTransform(s)
}

func (s *transformTestSuite) TestMetrics() {
	utils.TestMetricsTransform(s)
}

func (s *transformTestSuite) TestLogs() {
	utils.TestLogsTransform(s)
}

func (s *transformTestSuite) TestFlare() {
	utils.TestOTelFlareTransform(s)
}
