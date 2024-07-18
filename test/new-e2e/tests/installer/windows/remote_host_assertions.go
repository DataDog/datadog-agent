// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// RemoteHostAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost.
type RemoteHostAssertions struct {
	*SuiteAssertions
	remoteHost *components.RemoteHost
}

// HasAService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (r *RemoteHostAssertions) HasAService(serviceName string) *WindowsServiceAssertions {
	r.testing.Helper()
	serviceConfig, err := common.GetServiceConfig(r.remoteHost, serviceName)
	r.NoError(err)
	return &WindowsServiceAssertions{SuiteAssertions: r.SuiteAssertions, serviceConfig: serviceConfig}
}

// HasNoService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (r *RemoteHostAssertions) HasNoService(serviceName string) *RemoteHostAssertions {
	r.testing.Helper()
	_, err := common.GetServiceConfig(r.remoteHost, serviceName)
	r.Error(err)
	return r
}

// FileExists checks whether a file exists in the given path. It also fails if
// the path points to a directory or there is an error when trying to check the file.
func (r *RemoteHostAssertions) FileExists(path string, msgAndArgs ...interface{}) *RemoteHostAssertions {
	r.testing.Helper()
	exists, err := r.remoteHost.FileExists(path)
	r.NoError(err)
	r.True(exists, msgAndArgs...)
	return r
}

// HasARunningDatadogAgentService checks if the remote host has a Datadog Agent installed & running.
// It does not run a full test suite on it, but merely checks if it has the required
// service running.
func (r *RemoteHostAssertions) HasARunningDatadogAgentService() *RemoteHostAssertions {
	r.testing.Helper()
	r.FileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	r.HasAService("datadogagent").WithStatus("Running")
	return r
}

// HasNoDatadogAgentService checks if the remote host doesn't have a Datadog Agent installed.
func (r *RemoteHostAssertions) HasNoDatadogAgentService() *RemoteHostAssertions {
	r.testing.Helper()
	r.NoFileExists("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	r.HasNoService("datadogagent")
	return r
}

// HasBinary checks if a binary exists on the remote host and returns a more specific assertion
// allowing to run further tests on the binary.
func (r *RemoteHostAssertions) HasBinary(path string) *remoteBinaryAssertions {
	r.testing.Helper()
	r.FileExists(path)
	return &remoteBinaryAssertions{
		RemoteHostAssertions: r,
		binaryPath:           path,
	}
}
