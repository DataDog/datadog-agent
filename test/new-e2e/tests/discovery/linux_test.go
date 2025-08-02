// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

//go:embed testdata/config/agent_config.yaml
var agentConfigStr string

//go:embed testdata/config/system_probe_config.yaml
var systemProbeConfigStr string

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

var services = []string{
	"python-svc",
	"python-instrumented",
	"node-json-server",
	"node-instrumented",
	"rails-svc",
}

func TestLinuxTestSuite(t *testing.T) {
	t.Skip("Service Discovery E2E tests needs to be updated with new process pipeline")

	agentParams := []func(*agentparams.Params) error{
		agentparams.WithAgentConfig(agentConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr),
	}
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithAgentOptions(agentParams...))),
	}
	e2e.Run(t, &linuxTestSuite{}, options...)
}

func (s *linuxTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.provisionServer()
}

func (s *linuxTestSuite) TestServiceDiscoveryCheck() {
	t := s.T()
	s.startServices()
	defer s.stopServices()

	client := s.Env().FakeIntake.Client()
	err := client.FlushServerAndResetAggregators()
	require.NoError(t, err)

	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertRunningCheck(t, s.Env().RemoteHost, "service_discovery")
	}, 2*time.Minute, 10*time.Second)

	// This is very useful for debugging, but we probably don't want to decode
	// and assert based on this in this E2E test since this is an internal
	// interface between the agent and system-probe.
	services := s.Env().RemoteHost.MustExecute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/discovery/debug")
	t.Log("system-probe services", services)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := client.GetServiceDiscoveries()
		require.NoError(t, err)

		foundMap := make(map[string]*aggregator.ServiceDiscoveryPayload)
		for _, p := range payloads {
			name := p.Payload.GeneratedServiceName
			t.Log("RequestType", p.RequestType, "GeneratedServiceName", name)

			if p.RequestType == "start-service" {
				foundMap[name] = p
			}
		}

		s.assertService(t, c, foundMap, serviceExpectedPayload{
			systemdServiceName:   "node-json-server",
			instrumentation:      "none",
			generatedServiceName: "json-server",
			ddService:            "",
			serviceNameSource:    "",
		})
		s.assertService(t, c, foundMap, serviceExpectedPayload{
			systemdServiceName:   "node-instrumented",
			instrumentation:      "provided",
			generatedServiceName: "node-instrumented",
			ddService:            "",
			serviceNameSource:    "",
		})
		s.assertService(t, c, foundMap, serviceExpectedPayload{
			systemdServiceName:   "python-svc",
			instrumentation:      "none",
			generatedServiceName: "python.server",
			ddService:            "python-svc-dd",
			serviceNameSource:    "provided",
		})
		s.assertService(t, c, foundMap, serviceExpectedPayload{
			systemdServiceName:   "python-instrumented",
			instrumentation:      "provided",
			generatedServiceName: "python.instrumented",
			tracerServiceNames:   []string{"python-instrumented-dd"},
			ddService:            "python-instrumented-dd",
			serviceNameSource:    "provided",
		})
		s.assertService(t, c, foundMap, serviceExpectedPayload{
			systemdServiceName:   "rails-svc",
			instrumentation:      "none",
			generatedServiceName: "rails_hello",
			ddService:            "",
			serviceNameSource:    "",
		})

		assert.Contains(c, foundMap, "json-server")
	}, 3*time.Minute, 10*time.Second)
}

type checkStatus struct {
	CheckID           string `json:"CheckID"`
	CheckName         string `json:"CheckName"`
	CheckConfigSource string `json:"CheckConfigSource"`
	ExecutionTimes    []int  `json:"ExecutionTimes"`
}

type runnerStats struct {
	Checks map[string]checkStatus `json:"Checks"`
}

type collectorStatus struct {
	RunnerStats runnerStats `json:"runnerStats"`
}

func assertCollectorStatusFromJSON(t *assert.CollectT, statusOutput, check string) {
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(t, err, "failed to unmarshal agent status")

	assert.Contains(t, status.RunnerStats.Checks, check)
}

// assertRunningCheck asserts that the given process agent check is running
func assertRunningCheck(t *assert.CollectT, remoteHost *components.RemoteHost, check string) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")
	assertCollectorStatusFromJSON(t, statusOutput, check)
}

func (s *linuxTestSuite) provisionServer() {
	err := s.Env().RemoteHost.CopyFolder("testdata/provision", "/home/ubuntu/e2e-test")
	require.NoError(s.T(), err)

	cmd := "sudo bash /home/ubuntu/e2e-test/provision.sh"
	_, err = s.Env().RemoteHost.Execute(cmd)
	if err != nil {
		// Sometimes temporary network errors are seen which cause the provision
		// script to fail.
		s.T().Log("Retrying provision due to failure", err)
		time.Sleep(30 * time.Second)
		_, err := s.Env().RemoteHost.Execute(cmd)
		if err != nil {
			s.T().Skip("Unable to provision server")
		}
	}
}

func (s *linuxTestSuite) startServices() {
	for _, service := range services {
		s.Env().RemoteHost.MustExecute("sudo systemctl start " + service)
	}
}

func (s *linuxTestSuite) stopServices() {
	for i := len(services) - 1; i >= 0; i-- {
		service := services[i]
		s.Env().RemoteHost.MustExecute("sudo systemctl stop " + service)
	}
}

type serviceExpectedPayload struct {
	systemdServiceName   string
	instrumentation      string
	generatedServiceName string
	ddService            string
	serviceNameSource    string
	tracerServiceNames   []string
}

func (s *linuxTestSuite) assertService(t *testing.T, c *assert.CollectT, foundMap map[string]*aggregator.ServiceDiscoveryPayload, expected serviceExpectedPayload) {
	t.Helper()

	name := expected.generatedServiceName
	found := foundMap[name]
	if assert.NotNil(c, found, "could not find service %q", name) {
		assert.Equal(c, expected.instrumentation, found.Payload.APMInstrumentation, "service %q: APM instrumentation", name)
		assert.Equal(c, expected.generatedServiceName, found.Payload.GeneratedServiceName, "service %q: generated service name", name)
		assert.Equal(c, expected.ddService, found.Payload.DDService, "service %q: DD service", name)
		assert.Equal(c, expected.serviceNameSource, found.Payload.ServiceNameSource, "service %q: service name source", name)
		assert.NotZero(c, found.Payload.RSSMemory, "service %q: expected non-zero memory usage", name)
		if len(expected.tracerServiceNames) > 0 {
			var foundServiceNames []string
			var foundRuntimeIDs []string
			for _, tm := range found.Payload.TracerMetadata {
				foundServiceNames = append(foundServiceNames, tm.ServiceName)
				foundRuntimeIDs = append(foundRuntimeIDs, tm.RuntimeID)
			}
			assert.Equal(c, expected.tracerServiceNames, foundServiceNames, "service %q: tracer service names", name)
			assert.Len(c, foundRuntimeIDs, len(expected.tracerServiceNames), "service %q: tracer runtime ids", name)
		}
	} else {
		status := s.Env().RemoteHost.MustExecute("sudo systemctl status " + expected.systemdServiceName)
		logs := s.Env().RemoteHost.MustExecute("sudo journalctl -u " + expected.systemdServiceName)

		t.Logf("Service %q status:\n:%s", expected.systemdServiceName, status)
		t.Logf("Service %q logs:\n:%s", expected.systemdServiceName, logs)
	}
}
