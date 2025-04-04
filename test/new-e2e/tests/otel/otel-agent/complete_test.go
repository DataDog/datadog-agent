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
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type completeTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/complete.yml
var completeConfig string

func TestOTelAgentComplete(t *testing.T) {
	values := enableOTELAgentonfig(`
datadog:
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
          value: 'disable_receive_resource_spans_v2'
`)
	t.Parallel()
	e2e.Run(t, &completeTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithOTelAgent(), kubernetesagentparams.WithOTelConfig(completeConfig)))))
}

func (s *completeTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *completeTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, minimalParams)
}

func (s *completeTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, minimalParams)
}

func (s *completeTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, minimalParams)
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
