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
	s.testProcessCheckWithServiceDiscovery(agentProcessConfigStr, systemProbeConfigStr)
}

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscoveryProcessCollectionDisabled() {
	s.testProcessCheckWithServiceDiscovery(agentProcessDisabledConfigStr, systemProbeConfigStr)
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
		ok := t.Run(tc.description, func(t *testing.T) {
			var payloads []*aggregator.ProcessPayload
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				payloads, err = s.Env().FakeIntake.Client().GetProcesses()
				assert.NoError(c, err, "failed to get process payloads from fakeintake")
				// Wait for two payloads, as processes must be detected in two check runs to be returned
				assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")

				procs := process.FilterProcessPayloadsByName(payloads, tc.processName)
				assert.NotEmpty(c, procs, "'%s' process not found in payloads: \n%+v", tc.processName, payloads)
				assert.True(c, matchingProcessServiceDiscoveryData(procs, tc.expectedLanguage, tc.expectedPortInfo, tc.expectedService),
					"no process was found with the expected service discovery data. processes:\n%+v", procs)
				// processes that exist < 1 minute are ignored by service discovery and the service collection interval is 1 minute
				// therefore we should wait for 2 minutes to ensure service discovery is run at least once
				// start --> process collection, service discovery ignoring
				// 1 min --> process collection + service discovery collection ignores processes/may capture some
				// 2 min --> process collection + service discovery collection should capture everythinf
			}, 2*time.Minute, 10*time.Second)
		})
		if !ok {
			// This is very useful for debugging, but we probably don't want to decode
			// and assert based on this in this E2E test since this is an internal
			// interface between the agent and system-probe.
			discoveredServices := s.Env().RemoteHost.MustExecute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/discovery/debug")
			t.Log("system-probe services", discoveredServices)

			workloadmetaStore := s.Env().RemoteHost.MustExecute("sudo datadog-agent workload-list")
			t.Log("workloadmeta store", workloadmetaStore)
		}
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

// assertRunningCheck asserts that the given agent check is running
func assertRunningCheck(t *assert.CollectT, remoteHost *components.RemoteHost, check string) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")
	assertCollectorStatusFromJSON(t, statusOutput, check)
}

// assertNotRunningCheck asserts that the given agent check is not running
func assertNotRunningCheck(t *assert.CollectT, remoteHost *components.RemoteHost, check string) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(t, err, "failed to unmarshal agent status")
	assert.NotContains(t, status.RunnerStats.Checks, check)
}

// matchingProcessServiceDiscoveryData checks that the given processes contain at least 1 process with the expected service discovery data
// we cannot fail fast because many processes with the same name are not instrumented with service discovery data
func matchingProcessServiceDiscoveryData(procs []*agentmodel.Process, expectedLanguage agentmodel.Language, expectedPortInfo *agentmodel.PortInfo, expectedServiceDiscovery *agentmodel.ServiceDiscovery) bool {
	for _, proc := range procs {
		// check language
		if proc.Language != expectedLanguage {
			continue
		}

		// check port info
		if !matchingPortInfo(expectedPortInfo, proc.PortInfo) {
			continue
		}

		// check service discovery
		if expectedServiceDiscovery.ApmInstrumentation != proc.ServiceDiscovery.ApmInstrumentation {
			continue
		}

		if !matchingServiceName(expectedServiceDiscovery.DdServiceName, proc.ServiceDiscovery.DdServiceName) {
			continue
		}

		if !matchingServiceName(expectedServiceDiscovery.GeneratedServiceName, proc.ServiceDiscovery.GeneratedServiceName) {
			continue
		}

		if !matchingTracerMetadata(expectedServiceDiscovery.TracerMetadata, proc.ServiceDiscovery.TracerMetadata) {
			continue
		}

		if !matchingServiceNames(expectedServiceDiscovery.AdditionalGeneratedNames, proc.ServiceDiscovery.AdditionalGeneratedNames) {
			continue
		}
		return true
	}
	return false
}

func matchingServiceName(a, b *agentmodel.ServiceName) bool {
	return matchingServiceNames([]*agentmodel.ServiceName{a}, []*agentmodel.ServiceName{b})
}

func matchingServiceNames(expectedServiceNames []*agentmodel.ServiceName, actualServiceNames []*agentmodel.ServiceName) bool {
	// Sort by ServiceName so order doesn’t matter
	sort := cmpopts.SortSlices(func(a, b *agentmodel.ServiceName) bool {
		// handles cases where ServiceName is the same
		return a.Name != b.Name && a.Name < b.Name ||
			a.Source < b.Source
	})
	diff := cmp.Diff(expectedServiceNames, actualServiceNames, cmpopts.EquateEmpty(), sort)
	return diff == ""
}

func matchingPortInfo(expectedPortInfo *agentmodel.PortInfo, actualPortInfo *agentmodel.PortInfo) bool {
	if expectedPortInfo == nil {
		return actualPortInfo == nil
	} else if actualPortInfo == nil {
		// expectedPortInfo is not nil so actualPortInfo should not be
		return false
	}

	diffTCP := cmp.Diff(expectedPortInfo.Tcp, actualPortInfo.Tcp, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b int32) bool { return a < b }))
	diffUDP := cmp.Diff(expectedPortInfo.Udp, actualPortInfo.Udp, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b int32) bool { return a < b }))
	return diffTCP == "" && diffUDP == ""
}

func matchingTracerMetadata(expectedTracerMetadata []*agentmodel.TracerMetadata, actualTracerMetadata []*agentmodel.TracerMetadata) bool {
	// tracer metadata contains a uuid (TracerMetadata.RuntimeID), so we ignore it
	// Sort by ServiceName so order doesn’t matter
	sortByName := cmpopts.SortSlices(func(a, b *agentmodel.TracerMetadata) bool {
		// handles cases where ServiceName is the same
		return a.ServiceName != b.ServiceName && a.ServiceName < b.ServiceName ||
			a.RuntimeId < b.RuntimeId
	})

	// Ignore RuntimeID field completely
	ignoreID := cmpopts.IgnoreFields(agentmodel.TracerMetadata{}, "RuntimeId")

	diff := cmp.Diff(expectedTracerMetadata, actualTracerMetadata, cmpopts.EquateEmpty(), ignoreID, sortByName)
	return diff == ""
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
