// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
)

// RemoteHostAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost.
type RemoteHostAssertions struct {
	*SuiteAssertions
	remoteHost *components.RemoteHost
}

// HasAService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (remoteAssertions *RemoteHostAssertions) HasAService(serviceName string) *WindowsServiceAssertions {
	remoteAssertions.testing.Helper()
	serviceConfig, err := common.GetServiceConfig(remoteAssertions.remoteHost, serviceName)
	assert.NoError(remoteAssertions.testing, err)
	if err != nil {
		remoteAssertions.testing.FailNow()
	}
	return &WindowsServiceAssertions{SuiteAssertions: remoteAssertions.SuiteAssertions, serviceConfig: serviceConfig}
}

// HasNoService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (remoteAssertions *RemoteHostAssertions) HasNoService(serviceName string) *RemoteHostAssertions {
	remoteAssertions.testing.Helper()
	_, err := common.GetServiceConfig(remoteAssertions.remoteHost, serviceName)
	assert.Error(remoteAssertions.testing, err)
	return remoteAssertions
}

// FileExists checks whether a file exists in the given path. It also fails if
// the path points to a directory or there is an error when trying to check the file.
func (remoteAssertions *RemoteHostAssertions) FileExists(path string, msgAndArgs ...interface{}) *RemoteHostAssertions {
	remoteAssertions.testing.Helper()
	remoteAssertions.Host().FileExists(path, msgAndArgs...)
	return remoteAssertions
}

// HasADatadogAgent checks if the remote host has a Datadog Agent installed.
// It does not run a full test suite on it, but merely checks if it has the required
// service running.
func (remoteAssertions *RemoteHostAssertions) HasADatadogAgent() *RemoteHostAssertions {
	remoteAssertions.testing.Helper()
	remoteAssertions.FileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	remoteAssertions.HasAService("datadogagent").WithStatus("Running")
	return remoteAssertions
}

// HasNoDatadogAgent checks if the remote host doesn't have a Datadog Agent installed.
func (remoteAssertions *RemoteHostAssertions) HasNoDatadogAgent() *RemoteHostAssertions {
	remoteAssertions.testing.Helper()
	remoteAssertions.NoFileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	remoteAssertions.HasNoService("datadogagent")
	return remoteAssertions
}
