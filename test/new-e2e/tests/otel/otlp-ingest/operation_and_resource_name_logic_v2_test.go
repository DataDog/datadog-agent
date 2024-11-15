// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpingest

import (
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestOpAndResNameV2TestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	clientOperationName string
	clientResourceName  string
	serverOperationName string
	serverResourceName  string
}

func TestOpAndResNameV2WithSpanRecvrV1(t *testing.T) {
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
	ts := &otlpIngestOpAndResNameV2TestSuite{}
	ts.clientOperationName = "client.request"
	ts.clientResourceName = "lets-go"
	ts.serverOperationName = "server.request"
	ts.serverResourceName = "okey-dokey-0"
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func TestOpAndResNameV2WithSpanRecvrV2(t *testing.T) {
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
	ts := &otlpIngestOpAndResNameV2TestSuite{}
	ts.clientOperationName = "client.request"
	ts.clientResourceName = "lets-go"
	ts.serverOperationName = "server.request"
	ts.serverResourceName = "okey-dokey-0"
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func TestOpNameV2OverriddenBySpanAsResName(t *testing.T) {
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
	ts := &otlpIngestOpAndResNameV2TestSuite{}
	ts.clientOperationName = "lets_go"
	ts.clientResourceName = "lets-go"
	ts.serverOperationName = "okey_dokey_0"
	ts.serverResourceName = "okey-dokey-0"
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func TestOpNameV2OverriddenByRemapping(t *testing.T) {
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
	ts := &otlpIngestOpAndResNameV2TestSuite{}
	ts.clientOperationName = "mapping.output"
	ts.clientResourceName = "lets-go"
	ts.serverOperationName = "telemetrygen.server"
	ts.serverResourceName = "okey-dokey-0"
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithoutDualShipping(), kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpAndResNameV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.SetupSampleTraces(s)
}

func (s *otlpIngestOpAndResNameV2TestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, s.clientOperationName, s.clientResourceName, s.serverOperationName, s.serverResourceName)
}
