// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpingest

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestOpNameV2RecvrV1TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestOpNameV2SpanRecvrV1(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    traceAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'enable_operation_and_resource_name_logic_v2'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestOpNameV2RecvrV1TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RecvrV1TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestOpNameV2RecvrV1TestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "client.request", "lets-go", "server.request", "okey-dokey-0")
}

type otlpIngestOpNameV2RecvrV2TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestOpNameV2SpanRecvrV2(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    traceAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'enable_operation_and_resource_name_logic_v2,enable_receive_resource_spans_v2'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestOpNameV2RecvrV2TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RecvrV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestOpNameV2RecvrV2TestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "client.request", "lets-go", "server.request", "okey-dokey-0")
}

type otlpIngestOpNameV2SpanAsResNameTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestOpNameV2Override(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    traceAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'enable_operation_and_resource_name_logic_v2'
        - name: DD_OTLP_CONFIG_TRACES_SPAN_NAME_AS_RESOURCE_NAME
          value: 'true'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestOpNameV2SpanAsResNameTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2SpanAsResNameTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestOpNameV2SpanAsResNameTestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "lets_go", "lets-go", "okey_dokey_0", "okey-dokey-0")
}

type otlpIngestOpNameV2RemappingTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestOTLPIngestOpV2OverrideRemapping(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    traceAgent:
      env:
        - name: DD_APM_FEATURES
          value: 'enable_operation_and_resource_name_logic_v2'
        - name: DD_OTLP_CONFIG_TRACES_SPAN_NAME_REMAPPINGS
          value: '{"telemetrygen.client":"mapping.output","server.request":"telemetrygen.server"}'
`
	t.Parallel()
	ts := &otlpIngestOpNameV2RemappingTestSuite{}
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RemappingTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestOpNameV2RemappingTestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "mapping.output", "lets-go", "telemetrygen.server", "okey-dokey-0")
}
