// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package servicetest provides tests for the services installed by the Windows Agent
package servicetest

import (
	"fmt"
	"strings"

	infraCommon "github.com/DataDog/test-infra-definitions/common"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/stretchr/testify/assert"
)

// Tester provides tests for the services installed by the Windows Agent
type Tester struct {
	host *components.RemoteHost

	expectedUserName   string
	expectedUserDomain string

	// does not include the trailing backslash
	expectedInstallPath string
	expectedConfigRoot  string
}

// Option is a functional option for Tester
type Option = func(*Tester) error

// WithExpectedAgentUser sets the expected user for the Agent
func WithExpectedAgentUser(domain string, username string) Option {
	return func(t *Tester) error {
		t.expectedUserDomain = domain
		t.expectedUserName = username
		return nil
	}
}

// WithExpectedInstallPath sets the expected install path for the agent
func WithExpectedInstallPath(path string) Option {
	return func(t *Tester) error {
		t.expectedInstallPath = path
		return nil
	}
}

// WithExpectedConfigRoot sets the expected config root for the agent
func WithExpectedConfigRoot(path string) Option {
	return func(t *Tester) error {
		t.expectedConfigRoot = path
		return nil
	}
}

// NewTester creates a new Tester
func NewTester(host *components.RemoteHost, opts ...Option) (*Tester, error) {
	t := &Tester{host: host}

	// defaults
	t.expectedInstallPath = windowsAgent.DefaultInstallPath
	t.expectedConfigRoot = windowsAgent.DefaultConfigRoot

	_, err := infraCommon.ApplyOption(t, opts)
	if err != nil {
		return nil, err
	}

	// set this default after and only if needed since it requires some remote commands
	if t.expectedUserName == "" {
		hostInfo, err := windowsCommon.GetHostInfo(t.host)
		if err != nil {
			return nil, err
		}
		t.expectedUserName = windowsAgent.DefaultAgentUserName
		t.expectedUserDomain = windowsCommon.NameToNetBIOSName(hostInfo.Hostname)
	}

	// strip the trailing backslash for consistency
	t.expectedInstallPath = strings.TrimSuffix(t.expectedInstallPath, `\`)
	t.expectedConfigRoot = strings.TrimSuffix(t.expectedConfigRoot, `\`)

	return t, nil
}

// ExpectedServiceConfig returns the expected service config for the installed services
func (t *Tester) ExpectedServiceConfig() (windowsCommon.ServiceConfigMap, error) {
	m := windowsCommon.GetEmptyServiceConfigMap(ExpectedInstalledServices())

	// service type
	m["datadogagent"].ServiceType = windowsCommon.SERVICE_WIN32_OWN_PROCESS
	m["datadog-trace-agent"].ServiceType = windowsCommon.SERVICE_WIN32_OWN_PROCESS
	m["datadog-process-agent"].ServiceType = windowsCommon.SERVICE_WIN32_OWN_PROCESS
	m["datadog-security-agent"].ServiceType = windowsCommon.SERVICE_WIN32_OWN_PROCESS
	m["datadog-system-probe"].ServiceType = windowsCommon.SERVICE_WIN32_OWN_PROCESS
	m["ddnpm"].ServiceType = windowsCommon.SERVICE_KERNEL_DRIVER
	m["ddprocmon"].ServiceType = windowsCommon.SERVICE_KERNEL_DRIVER

	// start type
	m["datadogagent"].StartType = windowsCommon.SERVICE_AUTO_START
	m["datadog-trace-agent"].StartType = windowsCommon.SERVICE_DEMAND_START
	m["datadog-process-agent"].StartType = windowsCommon.SERVICE_DEMAND_START
	m["datadog-security-agent"].StartType = windowsCommon.SERVICE_DEMAND_START
	m["datadog-system-probe"].StartType = windowsCommon.SERVICE_DEMAND_START
	m["ddnpm"].StartType = windowsCommon.SERVICE_DISABLED
	m["ddprocmon"].StartType = windowsCommon.SERVICE_DISABLED

	// dependencies
	m["datadogagent"].ServicesDependedOn = []string{}
	m["datadog-trace-agent"].ServicesDependedOn = []string{"datadogagent"}
	m["datadog-process-agent"].ServicesDependedOn = []string{"datadogagent"}
	m["datadog-security-agent"].ServicesDependedOn = []string{"datadogagent"}
	m["datadog-system-probe"].ServicesDependedOn = []string{"datadogagent"}
	m["ddnpm"].ServicesDependedOn = []string{}
	m["ddprocmon"].ServicesDependedOn = []string{}

	// DisplayName
	m["datadogagent"].DisplayName = "Datadog Agent"
	m["datadog-trace-agent"].DisplayName = "Datadog Trace Agent"
	m["datadog-process-agent"].DisplayName = "Datadog Process Agent"
	m["datadog-security-agent"].DisplayName = "Datadog Security Service"
	m["datadog-system-probe"].DisplayName = "Datadog System Probe"
	m["ddnpm"].DisplayName = "Datadog Network Performance Monitor"
	m["ddprocmon"].DisplayName = "Datadog Process Monitor"

	// ImagePath
	exePath := quotePathIfContainsSpaces(fmt.Sprintf(`%s\bin\agent.exe`, t.expectedInstallPath))
	m["datadogagent"].ImagePath = exePath
	// TODO: double slash is intentional, must fix the path in the installer
	exePath = quotePathIfContainsSpaces(fmt.Sprintf(`%s\bin\agent\trace-agent.exe`, t.expectedInstallPath))
	m["datadog-trace-agent"].ImagePath = fmt.Sprintf(`%s --config="%s\\datadog.yaml"`, exePath, t.expectedConfigRoot)
	// TODO: double slash is intentional, must fix the path in the installer
	exePath = quotePathIfContainsSpaces(fmt.Sprintf(`%s\bin\agent\process-agent.exe`, t.expectedInstallPath))
	m["datadog-process-agent"].ImagePath = fmt.Sprintf(`%s --cfgpath="%s\\datadog.yaml"`, exePath, t.expectedConfigRoot)
	exePath = quotePathIfContainsSpaces(fmt.Sprintf(`%s\bin\agent\security-agent.exe`, t.expectedInstallPath))
	m["datadog-security-agent"].ImagePath = exePath
	exePath = quotePathIfContainsSpaces(fmt.Sprintf(`%s\bin\agent\system-probe.exe`, t.expectedInstallPath))
	m["datadog-system-probe"].ImagePath = exePath
	// drivers use the kernel path syntax and aren't quoted since they are file paths rather than command lines
	m["ddnpm"].ImagePath = fmt.Sprintf(`\??\%s\bin\agent\driver\ddnpm.sys`, t.expectedInstallPath)
	m["ddprocmon"].ImagePath = fmt.Sprintf(`\??\%s\bin\agent\driver\ddprocmon.sys`, t.expectedInstallPath)

	// User and SID
	expectedServiceUser := windowsCommon.MakeDownLevelLogonName(t.expectedUserDomain, t.expectedUserName)
	m["datadogagent"].UserName = expectedServiceUser
	m["datadog-trace-agent"].UserName = expectedServiceUser
	m["datadog-security-agent"].UserName = expectedServiceUser
	m["datadog-process-agent"].UserName = "LocalSystem"
	m["datadog-system-probe"].UserName = "LocalSystem"
	for _, s := range m {
		if !windowsCommon.IsUserModeServiceType(s.ServiceType) {
			continue
		}
		err := s.FetchUserSID(t.host)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

// ExpectedInstalledServices returns the list of services expected to be installed
func ExpectedInstalledServices() []string {
	return []string{
		"datadogagent",
		"datadog-trace-agent",
		"datadog-process-agent",
		"datadog-security-agent",
		"datadog-system-probe",
		"ddnpm",
		"ddprocmon",
	}
}

// quotePathIfContainsSpaces quotes the path if it contains spaces and returns the result.
//
// Due to how Windows parse the service command line, it is important to quote paths that contain spaces
// https://kb.cybertecsecurity.com/knowledge/unquoted-service-paths
func quotePathIfContainsSpaces(path string) string {
	if strings.Contains(path, " ") {
		return fmt.Sprintf(`"%s"`, path)
	}
	return path
}

// iterServiceConfigMaps iterates over the expected and actual service config maps and calls the provided function for each element.
// If the function returns false, the iteration stops and the function returns false.
// If an expected service is not found in the actual map, the function returns false.
func iterServiceConfigMaps(t *testing.T, expected windowsCommon.ServiceConfigMap, actual windowsCommon.ServiceConfigMap, f func(*windowsCommon.ServiceConfig, *windowsCommon.ServiceConfig) bool) bool {
	t.Helper()
	for name, e := range expected {
		a, ok := actual[name]
		if !assert.True(t, ok, "service %s not found", name) {
			return false
		}
		if !f(e, a) {
			return false
		}
	}
	return true
}

// AssertEqualServiceConfigValues asserts that the service config values from the expected map match the actual map
func AssertEqualServiceConfigValues(t *testing.T, expected windowsCommon.ServiceConfigMap, actual windowsCommon.ServiceConfigMap) bool {
	return iterServiceConfigMaps(t, expected, actual, func(expected *windowsCommon.ServiceConfig, actual *windowsCommon.ServiceConfig) bool {
		assert.Equal(t, expected.DisplayName, actual.DisplayName, "service %s DisplayName should match", actual.ServiceName)
		assert.Equal(t, expected.ImagePath, actual.ImagePath, "service %s ImagePath should match", actual.ServiceName)
		assert.Equal(t, expected.StartType, actual.StartType, "service %s StartType should match", actual.ServiceName)
		assert.Equal(t, expected.ServiceType, actual.ServiceType, "service %s ServiceType should match", actual.ServiceName)
		// Compare UserSID rather than UserNames to avoid needing to handle name formatting differences
		assert.Equal(t, expected.UserSID, actual.UserSID, "service %s user should be (%s,%s)", actual.ServiceName, expected.UserName, expected.UserSID)
		assert.ElementsMatch(t, expected.ServicesDependedOn, actual.ServicesDependedOn, "service %s ServicesDependedOn should match", actual.ServiceName)
		// continue iterating regardless of the result
		return true
	})
}
