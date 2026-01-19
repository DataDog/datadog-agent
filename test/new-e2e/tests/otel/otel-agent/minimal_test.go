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

type minimalTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/minimal.yml
var minimalConfig string

//go:embed testdata/minimal-provided-config.yml
var minimalProvidedConfig string

//go:embed testdata/minimal-full-config.yml
var minimalFullConfig string

//go:embed testdata/minimal-full-config-datadogextension.yml
var minimalFullConfigDatadogExtension string

//go:embed testdata/sources.json
var sources string

func TestOTelAgentMinimal(t *testing.T) {
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
	e2e.Run(t, &minimalTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(minimalConfig),
				),
			),
		)),
	)
}

var minimalParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.LowCardinality,
}

func (s *minimalTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *minimalTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, minimalParams)
}

func (s *minimalTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, minimalParams)
}

func (s *minimalTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, minimalParams)
}

func (s *minimalTestSuite) TestHosts() {
	utils.TestHosts(s)
}

func (s *minimalTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}

func (s *minimalTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *minimalTestSuite) TestOTelFlareExtensionResponse() {
	utils.TestOTelFlareExtensionResponse(s, minimalProvidedConfig, minimalFullConfig, sources)
}

func (s *minimalTestSuite) TestOTelFlareFiles() {
	utils.TestOTelFlareFiles(s)
}

func (s *minimalTestSuite) TestDatadogExtensionPayload() {
	utils.TestDatadogExtensionPayload(s, minimalFullConfigDatadogExtension)
}

func (s *minimalTestSuite) TestCoreAgentStatus() {
	utils.TestCoreAgentStatusCmd(s)
}

func (s *minimalTestSuite) TestOTelAgentStatus() {
	utils.TestOTelAgentStatusCmd(s)
}

func (s *minimalTestSuite) TestOTelAgentFlare() {
	utils.TestOTelAgentFlareCmd(s)
}

func (s *minimalTestSuite) TestCoreAgentConfigCmd() {
	const expectedCfg = `service:
  extensions:
  - pprof/dd-autoconfigured
  - zpages/dd-autoconfigured
  - health_check/dd-autoconfigured
  - ddflare/dd-autoconfigured
  - datadog/dd-autoconfigured
  pipelines:
    logs:
      exporters:
      - datadog
      processors:
      - infraattributes/dd-autoconfigured
      receivers:
      - otlp
    metrics:
      exporters:
      - datadog
      processors:
      - infraattributes/dd-autoconfigured
      receivers:
      - otlp
      - datadog/connector
    metrics/dd-autoconfigured/datadog:
      exporters:
      - datadog
      processors:
      - filter/drop-prometheus-internal-metrics/dd-autoconfigured
      receivers:
      - prometheus/dd-autoconfigured
    traces:
      exporters:
      - datadog/connector
      processors:
      - infraattributes/dd-autoconfigured
      receivers:
      - otlp
    traces/send:
      exporters:
      - datadog
      processors:
      - infraattributes/dd-autoconfigured
      receivers:
      - otlp`
	utils.TestCoreAgentConfigCmd(s, expectedCfg)
}
