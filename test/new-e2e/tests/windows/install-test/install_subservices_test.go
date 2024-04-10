// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"strconv"
	"time"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type subservicesTestCase struct {
	name string
	// it's surprising but we do not have an installer option for enabling NPM/system-probe.
	logsEnabled    bool
	processEnabled bool
	apmEnabled     bool
}

func TestSubServicesOpts(t *testing.T) {
	tcs := []subservicesTestCase{
		// TC-INS-004
		{"all-subservices", true, true, true},
		// TC-INS-005
		{"no-subservices", false, false, false},
	}
	for _, tc := range tcs {
		s := &testSubServicesOptsSuite{
			tc: tc,
		}
		t.Run(tc.name, func(t *testing.T) {
			run(t, s)
		})
		// clean the host between test runs
		s.cleanupOnSuccessInDevMode()
	}
}

type testSubServicesOptsSuite struct {
	baseAgentMSISuite

	tc subservicesTestCase
}

// TestSubServicesOpts tests that the agent installer can configure the subservices.
// TODO: Once E2E's Agent interface supports providing MSI installer options these tests
// should be moved to regular Agent E2E tests for each subservice.
func (s *testSubServicesOptsSuite) TestSubServicesOpts() {
	vm := s.Env().RemoteHost
	tc := s.tc

	installOpts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithLogsEnabled(strconv.FormatBool(tc.logsEnabled)),
		// set both process agent options so we can check if process-agent is running or not
		windowsAgent.WithProcessEnabled(strconv.FormatBool(tc.processEnabled)),
		windowsAgent.WithProcessDiscoveryEnabled(strconv.FormatBool(tc.processEnabled)),
		windowsAgent.WithAPMEnabled(strconv.FormatBool(tc.apmEnabled)),
	}
	_ = s.installAgentPackage(vm, s.AgentPackage, installOpts...)

	// read the config file and check the options
	confYaml, err := s.readAgentConfig(vm)
	s.Require().NoError(err)

	assert.Contains(s.T(), confYaml, "logs_enabled", "logs_enabled should be present in the config")
	assert.Equal(s.T(), tc.logsEnabled, confYaml["logs_enabled"], "logs_enabled should match")

	if assert.Contains(s.T(), confYaml, "process_config", "process_config should be present in the config") {
		processConf := confYaml["process_config"].(map[string]interface{})
		if assert.Contains(s.T(), processConf, "process_collection", "process_collection should be present in process_config") {
			processCollectionConf := processConf["process_collection"].(map[string]interface{})
			assert.Contains(s.T(), processCollectionConf, "enabled", "enabled should be present in process_collection")
			assert.Equal(s.T(), tc.processEnabled, processCollectionConf["enabled"], "process_collection enabled should match")
		}
		if assert.Contains(s.T(), processConf, "process_discovery", "process_discovery should be present in process_config") {
			processDiscoveryConf := processConf["process_discovery"].(map[string]interface{})
			assert.Contains(s.T(), processDiscoveryConf, "enabled", "enabled should be present in process_discovery")
			assert.Equal(s.T(), tc.processEnabled, processDiscoveryConf["enabled"], "process_discovery enabled should match")
		}
	}

	if assert.Contains(s.T(), confYaml, "apm_config", "apm_config should be present in the config") {
		apmConf := confYaml["apm_config"].(map[string]interface{})
		assert.Contains(s.T(), apmConf, "enabled", "enabled should be present in apm_config")
		assert.Equal(s.T(), tc.apmEnabled, apmConf["enabled"], "apm_config enabled should match")
	}

	tcs := []struct {
		serviceName string
		enabled     bool
	}{
		// NOTE: Even with processEnabled=false the Agent will start process-agent because container_collection is
		//       enabled by default. We do not have an installer option to control this process-agent setting.
		//       However, process-agent will exit soon after starting because there's no container environment installed
		//       and the other options are disabled.
		{"datadog-process-agent", tc.processEnabled},
		{"datadog-trace-agent", tc.apmEnabled},
	}
	for _, tc := range tcs {
		assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(vm, tc.serviceName)
			require.NoError(c, err)
			if tc.enabled {
				assert.Equal(c, "Running", status, "%s should be running", tc.serviceName)
			} else {
				assert.Equal(c, "Stopped", status, "%s should be stopped", tc.serviceName)
			}
		}, 1*time.Minute, 1*time.Second, "%s should be in the expected state", tc.serviceName)
	}
}
