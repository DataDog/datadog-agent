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
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
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
	e2e.Run(t, &otlpIngestOpNameV2RecvrV1TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RecvrV1TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestOpNameV2RecvrV1TestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "client.request", "getDate", "http.server.request", "GET")
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
          value: 'enable_operation_and_resource_name_logic_v2'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestOpNameV2RecvrV2TestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RecvrV2TestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestOpNameV2RecvrV2TestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "client.request", "getDate", "http.server.request", "GET")
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
	e2e.Run(t, &otlpIngestOpNameV2SpanAsResNameTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2SpanAsResNameTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestOpNameV2SpanAsResNameTestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "getDate", "getDate", "CalendarHandler", "GET")
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
          value: '{"calendar-rest-go.client":"mapping.output","go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp.server":"calendar.server"}'
`
	t.Parallel()
	ts := &otlpIngestOpNameV2RemappingTestSuite{}
	e2e.Run(t, ts, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values)))))
}

func (s *otlpIngestOpNameV2RemappingTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestOpNameV2RemappingTestSuite) TestTraces() {
	utils.TestTracesWithOperationAndResourceName(s, "mapping.output", "getDate", "calendar.server", "GET")
}
