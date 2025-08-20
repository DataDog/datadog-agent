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

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/process"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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

//go:embed testdata/config/agent_process_config.yaml
var agentProcessConfigStr string

//go:embed testdata/config/agent_process_disabled_config.yaml
var agentProcessDisabledConfigStr string

//go:embed testdata/config/system_probe_process_config.yaml
var systemProbeProcessConfigStr string

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
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

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

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscovery() {
	s.testProcessCheckWithServiceDiscovery(agentProcessConfigStr, systemProbeProcessConfigStr)
}

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscoveryProcessCollectionDisabled() {
	s.testProcessCheckWithServiceDiscovery(agentProcessDisabledConfigStr, systemProbeProcessConfigStr)
}

func (s *linuxTestSuite) testProcessCheckWithServiceDiscovery(agentConfigStr string, systemProbeConfigStr string) {
	t := s.T()
	s.startServices()
	defer s.stopServices()
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithAgentConfig(agentConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr))),
	)
	client := s.Env().FakeIntake.Client()
	err := client.FlushServerAndResetAggregators()
	require.NoError(t, err)

	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertNotRunningCheck(t, s.Env().RemoteHost, "service_discovery")
	}, 1*time.Minute, 10*time.Second)

	for _, tc := range []struct {
		description      string
		processName      string
		runningService   string
		expectedLanguage agentmodel.Language
		expectedPortInfo *agentmodel.PortInfo
		expectedService  *agentmodel.ServiceDiscovery
	}{
		{
			description:      "node-json-server",
			processName:      "node",
			expectedLanguage: agentmodel.Language_LANGUAGE_NODE,
			expectedPortInfo: &agentmodel.PortInfo{
				Tcp: []int32{8084},
			},
			expectedService: &agentmodel.ServiceDiscovery{
				GeneratedServiceName: &agentmodel.ServiceName{
					Name:   "json-server",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
				},
			},
		},
		{
			description:      "node-instrumented",
			processName:      "node",
			expectedLanguage: agentmodel.Language_LANGUAGE_NODE,
			expectedPortInfo: &agentmodel.PortInfo{
				Tcp: []int32{8085},
			},
			expectedService: &agentmodel.ServiceDiscovery{
				ApmInstrumentation: true,
				GeneratedServiceName: &agentmodel.ServiceName{
					Name:   "node-instrumented",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
				},
				TracerMetadata: []*agentmodel.TracerMetadata{
					{
						ServiceName: "node-instrumented",
					},
				},
			},
		},
		{
			description:      "python-svc",
			processName:      "/usr/bin/python3",
			expectedLanguage: agentmodel.Language_LANGUAGE_PYTHON,
			expectedPortInfo: &agentmodel.PortInfo{
				Tcp: []int32{8082},
			},
			expectedService: &agentmodel.ServiceDiscovery{
				GeneratedServiceName: &agentmodel.ServiceName{
					Name:   "python.server",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
				},
				DdServiceName: &agentmodel.ServiceName{
					Name:   "python-svc-dd",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
				},
			},
		},
		{
			description:      "python-instrumented",
			processName:      "/usr/bin/python3",
			expectedLanguage: agentmodel.Language_LANGUAGE_PYTHON,
			expectedPortInfo: &agentmodel.PortInfo{
				Tcp: []int32{8083},
			},
			expectedService: &agentmodel.ServiceDiscovery{
				ApmInstrumentation: true,
				GeneratedServiceName: &agentmodel.ServiceName{
					Name:   "python.instrumented",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
				},
				DdServiceName: &agentmodel.ServiceName{
					Name:   "python-instrumented-dd",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
				},
				TracerMetadata: []*agentmodel.TracerMetadata{
					{
						ServiceName: "python-instrumented-dd",
					},
				},
			},
		},
		{
			description:      "rails-svc",
			processName:      "ruby3.0",
			expectedLanguage: agentmodel.Language_LANGUAGE_RUBY,
			expectedPortInfo: &agentmodel.PortInfo{
				Tcp: []int32{7777},
			},
			expectedService: &agentmodel.ServiceDiscovery{
				GeneratedServiceName: &agentmodel.ServiceName{
					Name:   "rails_hello",
					Source: agentmodel.ServiceNameSource_SERVICE_NAME_SOURCE_RAILS,
				},
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			var payloads []*aggregator.ProcessPayload
			ok := assert.EventuallyWithT(t, func(c *assert.CollectT) {
				payloads, err = s.Env().FakeIntake.Client().GetProcesses()
				assert.NoError(c, err, "failed to get process payloads from fakeintake")
				// Wait for two payloads, as processes must be detected in two check runs to be returned
				assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

				procs := process.FilterProcessPayloadsByName(payloads, tc.processName)
				assert.NotEmpty(c, procs, "'%s' process not found in payloads: \n%+v", tc.processName, payloads)
				assertProcessServiceDiscoveryData(t, c, procs, tc.expectedLanguage, tc.expectedPortInfo, tc.expectedService)
			}, 2*time.Minute, 10*time.Second)
			if !ok {
				t.Logf("process payloads: %+v", payloads)
				// This is very useful for debugging, but we probably don't want to decode
				// and assert based on this in this E2E test since this is an internal
				// interface between the agent and system-probe.
				discoveredServices := s.Env().RemoteHost.MustExecute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/discovery/debug")
				t.Log("system-probe services", discoveredServices)
			}
		})
	}

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

// assertNotRunningCheck asserts that the given process agent check is not running
func assertNotRunningCheck(t *assert.CollectT, remoteHost *components.RemoteHost, check string) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(t, err, "failed to unmarshal agent status")
	assert.NotContains(t, status.RunnerStats.Checks, check)
}

func assertProcessServiceDiscoveryData(t *testing.T, c *assert.CollectT, procs []*agentmodel.Process, expectedLanguage agentmodel.Language, expectedPortInfo *agentmodel.PortInfo, expectedServiceDiscovery *agentmodel.ServiceDiscovery) {
	t.Helper()

	var hasServiceDiscovery bool
	// verify there is a process with the right service discovery data populated
	for _, proc := range procs {
		// check language
		correctLanguage := proc.Language == expectedLanguage

		// check port info
		var correctPortInfo bool
		if expectedPortInfo != nil {
			if proc.PortInfo == nil {
				continue
			}
			correctPortInfo = assert.ElementsMatch(noopt{}, expectedPortInfo.Tcp, proc.PortInfo.Tcp) && assert.ElementsMatch(noopt{}, expectedPortInfo.Udp, proc.PortInfo.Udp)
		} else {
			correctPortInfo = proc.PortInfo == nil
		}

		// check service discovery
		correctServiceDiscovery := expectedServiceDiscovery.ApmInstrumentation == proc.ServiceDiscovery.ApmInstrumentation

		if expectedServiceDiscovery.DdServiceName != nil {
			if proc.ServiceDiscovery.DdServiceName == nil {
				continue
			}
			correctServiceDiscovery = correctServiceDiscovery && expectedServiceDiscovery.DdServiceName.Name == proc.ServiceDiscovery.DdServiceName.Name &&
				expectedServiceDiscovery.DdServiceName.Source == proc.ServiceDiscovery.DdServiceName.Source
		} else {
			correctServiceDiscovery = correctServiceDiscovery && proc.ServiceDiscovery.DdServiceName == nil
		}

		if expectedServiceDiscovery.GeneratedServiceName != nil {
			if proc.ServiceDiscovery.GeneratedServiceName == nil {
				continue
			}
			correctServiceDiscovery = correctServiceDiscovery && expectedServiceDiscovery.GeneratedServiceName.Name == proc.ServiceDiscovery.GeneratedServiceName.Name &&
				expectedServiceDiscovery.GeneratedServiceName.Source == proc.ServiceDiscovery.GeneratedServiceName.Source
		} else {
			correctServiceDiscovery = correctServiceDiscovery && proc.ServiceDiscovery.GeneratedServiceName == nil
		}

		if len(expectedServiceDiscovery.TracerMetadata) > 0 {
			if len(proc.ServiceDiscovery.TracerMetadata) == 0 {
				continue
			}
			// tracer metadata contains a uuid (TracerMetadata.RuntimeID), so we do a manual comparison here
			//correctServiceDiscovery = correctServiceDiscovery && assert.ElementsMatch(noopt{}, expectedServiceDiscovery.TracerMetadata, proc.ServiceDiscovery.TracerMetadata)
			// Sort by ServiceName so order doesn’t matter
			sortByName := cmpopts.SortSlices(func(x, y *agentmodel.TracerMetadata) bool {
				return x.ServiceName < y.ServiceName
			})

			// Ignore RuntimeID field completely
			ignoreID := cmpopts.IgnoreFields(agentmodel.TracerMetadata{}, "RuntimeId")

			diff := cmp.Diff(proc.ServiceDiscovery.TracerMetadata, expectedServiceDiscovery.TracerMetadata, sortByName, ignoreID)
			correctServiceDiscovery = correctServiceDiscovery && diff == ""
		} else {
			correctServiceDiscovery = correctServiceDiscovery && len(proc.ServiceDiscovery.TracerMetadata) == 0
		}

		if len(expectedServiceDiscovery.AdditionalGeneratedNames) > 0 {
			if len(proc.ServiceDiscovery.AdditionalGeneratedNames) == 0 {
				continue
			}
			correctServiceDiscovery = correctServiceDiscovery && assert.ElementsMatch(noopt{}, expectedServiceDiscovery.AdditionalGeneratedNames, proc.ServiceDiscovery.AdditionalGeneratedNames)
		} else {
			correctServiceDiscovery = correctServiceDiscovery && len(proc.ServiceDiscovery.AdditionalGeneratedNames) == 0
		}

		hasServiceDiscovery = correctPortInfo && correctLanguage && correctServiceDiscovery
		if hasServiceDiscovery {
			break
		}
	}
	assert.True(c, hasServiceDiscovery, "no process was found with expected service discovery data")
}

type noopt struct{}

func (t noopt) Errorf(string, ...interface{}) {}

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
