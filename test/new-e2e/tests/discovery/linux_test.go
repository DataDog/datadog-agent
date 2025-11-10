// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/process"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/config/agent_config.yaml
var agentConfigStr string

//go:embed testdata/config/system_probe_config.yaml
var systemProbeConfigStr string

//go:embed testdata/config/system_probe_config_privileged_logs.yaml
var systemProbeConfigPrivilegedLogsStr string

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
	"python-restricted",
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
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentParams...)))),
	}
	e2e.Run(t, &linuxTestSuite{}, options...)
}

func (s *linuxTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	s.provisionServer()
}

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscovery() {
	s.testProcessCheckWithServiceDiscovery(agentProcessConfigStr, systemProbeConfigStr)
}

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscoveryProcessCollectionDisabled() {
	s.testProcessCheckWithServiceDiscovery(agentProcessDisabledConfigStr, systemProbeConfigStr)
}

func (s *linuxTestSuite) TestProcessCheckWithServiceDiscoveryPrivilegedLogs() {
	servicesWithRestricted := []string{
		"python-restricted",
	}
	s.testProcessCheckWithServiceDiscoveryPrivilegedLogs(agentProcessConfigStr, systemProbeConfigPrivilegedLogsStr, servicesWithRestricted)
}

func (s *linuxTestSuite) testLogs(t *testing.T) {
	s.Env().RemoteHost.MustExecute("curl -s http://localhost:8082/test")

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("python-svc-dd")
		assert.NoError(c, err, "failed to get logs from fakeintake")

		assert.NotEmpty(c, logs, "Expected to find logs from python-svc-dd service")

		foundStartupLog := false
		foundRequestLog := false

		for _, log := range logs {
			// Print unconditionally since the E2E tests are mostly run in CI
			// and the extra messages shouldn't be too noisy.
			t.Logf("Log: %+v", log)
			assert.Equal(c, "python-svc-dd", log.Service, "Log service should match")
			assert.Equal(c, "python", log.Source, "Log source should match")

			assert.Contains(c, log.Tags, "service:python-svc-dd")
			assert.Contains(c, log.Tags, "version:2.1")
			assert.Contains(c, log.Tags, "env:prod")

			if log.Message == "Server is running on http://0.0.0.0:8082" {
				foundStartupLog = true
			}
			if log.Message == "GET /test" {
				foundRequestLog = true
			}
		}

		assert.True(c, foundStartupLog, "Should find startup log message")
		assert.True(c, foundRequestLog, "Should find request log message")
	}, 2*time.Minute, 10*time.Second)

	// Verify discovery check reports permission warnings for restricted log
	// files.  There is no fake intake for the check status and the check status
	// is also sent out once every 10 minutes, so check the agent status
	// instead, which should be good enough.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		warnings, err := getDiscoveryCheckWarnings(t, s.Env().RemoteHost)
		assert.NoError(c, err, "failed to get discovery check warnings from agent status")

		foundPermissionWarning := false

		for _, warning := range warnings {
			t.Logf("Discovery warning: type=%s, error_code=%s, resource=%s, message=%s",
				warning.Type, warning.ErrorCode, warning.Resource, warning.Message)

			if warning.Type == "log_file" &&
				warning.ErrorCode == "permission-denied" &&
				strings.HasPrefix(warning.Resource, "/tmp/python-restricted") &&
				warning.ErrorString != "" &&
				warning.Message != "" {
				foundPermissionWarning = true
				break
			}
		}

		assert.True(c, foundPermissionWarning, "Should find permission warning for restricted log file")
	}, 2*time.Minute, 10*time.Second)
}

func (s *linuxTestSuite) dumpDebugInfo(t *testing.T) {
	// This is very useful for debugging, but we probably don't want to decode
	// and assert based on this in this E2E test since this is an internal
	// interface between the agent and system-probe.
	discoveredServices := s.Env().RemoteHost.MustExecute("sudo curl -s --unix /opt/datadog-agent/run/sysprobe.sock http://unix/discovery/debug")
	t.Log("system-probe services", discoveredServices)

	workloadmetaStore := s.Env().RemoteHost.MustExecute("sudo datadog-agent workload-list --verbose")
	t.Log("workloadmeta store", workloadmetaStore)

	status := s.Env().RemoteHost.MustExecute("sudo datadog-agent status")
	t.Log("agent status", status)
}

func (s *linuxTestSuite) testProcessCheckWithServiceDiscovery(agentConfigStr string, systemProbeConfigStr string) {
	t := s.T()
	s.startServices()
	defer s.stopServices()
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(
		scenec2.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfigStr),
			agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	)),
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
				// 2 min --> process collection + service discovery collection should capture everything
				// 3 min --> extra time for the collected data to actually be sent by the process check
			}, 3*time.Minute, 10*time.Second)
		})
		if !ok {
			s.dumpDebugInfo(t)
		}
	}

	ok := t.Run("logs", s.testLogs)
	if !ok {
		s.dumpDebugInfo(t)
	}
}

func (s *linuxTestSuite) testProcessCheckWithServiceDiscoveryPrivilegedLogs(agentConfigStr string, systemProbeConfigStr string, servicesToStart []string) {
	t := s.T()
	s.startServicesFromList(servicesToStart)
	defer s.stopServicesFromList(servicesToStart)
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(
		scenec2.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfigStr),
			agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	)),
	)
	client := s.Env().FakeIntake.Client()
	err := client.FlushServerAndResetAggregators()
	require.NoError(t, err)

	// Trigger log generation in the restricted service
	s.Env().RemoteHost.MustExecute("curl -s http://localhost:8086/test")

	ok := assert.EventuallyWithT(t, func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("python-restricted-dd")
		assert.NoError(c, err, "failed to get logs from fakeintake")

		assert.NotEmpty(c, logs, "Expected to find logs from python-restricted-dd service")

		foundStartupLog := false
		foundRequestLog := false

		for _, log := range logs {
			t.Logf("Log: %+v", log)
			assert.Equal(c, "python-restricted-dd", log.Service, "Log service should match")
			assert.Equal(c, "python", log.Source, "Log source should match")

			if log.Message == "Server is running on http://0.0.0.0:8086" {
				foundStartupLog = true
			}
			if log.Message == "GET /test" {
				foundRequestLog = true
			}
		}

		assert.True(c, foundStartupLog, "Should find startup log message")
		assert.True(c, foundRequestLog, "Should find request log message")
		// Wait for more time than the normal test since here we don't wait for the process collection first.
	}, 4*time.Minute, 10*time.Second)

	if !ok {
		s.dumpDebugInfo(t)
	}
}

type checkStatus struct {
	CheckID           string   `json:"CheckID"`
	CheckName         string   `json:"CheckName"`
	CheckConfigSource string   `json:"CheckConfigSource"`
	ExecutionTimes    []int    `json:"ExecutionTimes"`
	LastWarnings      []string `json:"LastWarnings"`
}

type (
	checkName    = string
	instanceName = string
	runnerStats  struct {
		Checks map[checkName]map[instanceName]checkStatus `json:"Checks"`
	}
)

type collectorStatus struct {
	RunnerStats runnerStats `json:"runnerStats"`
}

// Warning represents a structured warning from discovery check
type Warning struct {
	Type        string `json:"type"`
	Version     int    `json:"version"`
	Resource    string `json:"resource"`
	ErrorCode   string `json:"error_code"`
	ErrorString string `json:"error_string"`
	Message     string `json:"message"`
}

// getDiscoveryCheckWarnings parses discovery check warnings from agent status
func getDiscoveryCheckWarnings(t *testing.T, remoteHost *components.RemoteHost) ([]Warning, error) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if err != nil {
		return nil, err
	}

	instances, exists := status.RunnerStats.Checks["discovery"]
	if !exists {
		return []Warning{}, nil
	}

	discoveryCheck, exists := instances["discovery"]
	if !exists {
		return []Warning{}, nil
	}

	t.Logf("Discovery check warnings: %+v", discoveryCheck.LastWarnings)

	var warnings []Warning
	for _, warningStr := range discoveryCheck.LastWarnings {
		var warning Warning
		if err := json.Unmarshal([]byte(warningStr), &warning); err == nil {
			warnings = append(warnings, warning)
		}
	}

	return warnings, nil
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
	s.startServicesFromList(services)
}

func (s *linuxTestSuite) stopServices() {
	s.stopServicesFromList(services)
}

func (s *linuxTestSuite) startServicesFromList(servicesList []string) {
	for _, service := range servicesList {
		s.Env().RemoteHost.MustExecute("sudo systemctl start " + service)
	}
}

func (s *linuxTestSuite) stopServicesFromList(servicesList []string) {
	for i := len(servicesList) - 1; i >= 0; i-- {
		service := servicesList[i]
		s.Env().RemoteHost.MustExecute("sudo systemctl stop " + service)
	}
}
