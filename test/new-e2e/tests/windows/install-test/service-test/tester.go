// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package servicetest provides tests for the services installed by the Windows Agent
package servicetest

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	infraCommon "github.com/DataDog/test-infra-definitions/common"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tester provides tests for the services installed by the Windows Agent
type Tester struct {
	host *components.RemoteHost

	expectedAgentUserName   string
	expectedAgentUserDomain string
}

// Option is a functional option for Tester
type Option = func(*Tester) error

// WithExpectedAgentUser sets the expected user for the Agent
func WithExpectedAgentUser(domain string, username string) Option {
	return func(t *Tester) error {
		t.expectedAgentUserDomain = domain
		t.expectedAgentUserName = username
		return nil
	}
}

// NewTester creates a new Tester
func NewTester(host *components.RemoteHost, opts ...Option) (*Tester, error) {
	t := &Tester{host: host}
	_, err := infraCommon.ApplyOption(t, opts)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Tester) expectedServiceUsers() (windowsCommon.ServiceConfigMap, error) {
	expectedServiceUser := windowsCommon.MakeDownLevelLogonName(t.expectedAgentUserDomain, t.expectedAgentUserName)

	m := windowsCommon.GetEmptyServiceConfigMap(t.expectedInstalledServices())
	m["DatadogAgent"].UserName = expectedServiceUser
	m["datadog-trace-agent"].UserName = expectedServiceUser
	m["datadog-process-agent"].UserName = "LocalSystem"
	m["datadog-system-probe"].UserName = "LocalSystem"
	for _, s := range m {
		err := s.FetchUserSID(t.host)
		if err != nil {
			return nil, err
		}
	}

	return m, nil
}

func (t *Tester) expectedInstalledServices() []string {
	return []string{
		"DatadogAgent",
		"datadog-trace-agent",
		"datadog-process-agent",
		"datadog-system-probe",
	}
}

// TestInstall tests the expectations for the installed services
func (t *Tester) TestInstall(tt *testing.T) bool {
	return tt.Run("service config", func(tt *testing.T) {
		tt.Run("service users", func(tt *testing.T) {
			actual, err := windowsCommon.GetServiceConfigMap(t.host, t.expectedInstalledServices())
			require.NoError(tt, err)
			expectedServiceUsers, err := t.expectedServiceUsers()
			require.NoError(tt, err)
			AssertServiceUsers(tt, expectedServiceUsers, actual)
		})
	})

}

// iterServiceConfigMaps iterates over the expected and actual service config maps and calls the provided function for each element.
// If the function returns false, the iteration stops and the function returns false.
// If an expected service is not found in the actual map, the function returns false.
func iterServiceConfigMaps(t *testing.T, expected windowsCommon.ServiceConfigMap, actual windowsCommon.ServiceConfigMap, f func(*windowsCommon.ServiceConfig, *windowsCommon.ServiceConfig) bool) bool {
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

// AssertServiceUsers asserts that the service users from the expected map match the actual map
//
// Compares UserSIDs rather than UserNames to avoid needing to handle name formatting differences
func AssertServiceUsers(t *testing.T, expected windowsCommon.ServiceConfigMap, actual windowsCommon.ServiceConfigMap) bool {
	return iterServiceConfigMaps(t, expected, actual, func(expected *windowsCommon.ServiceConfig, actual *windowsCommon.ServiceConfig) bool {
		return assert.Equal(t, expected.UserSID, actual.UserSID, "service %s user should be (%s,%s)", actual.ServiceName, expected.UserName, expected.UserSID)
	})
}
