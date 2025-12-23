// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedlibrary contains tests for the shared library checks
package sharedlibrary

import (
	_ "embed"
	"path"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	osVM "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	checkutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed files/conf.yaml
var exampleCheckConfig string

type sharedLibrarySuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor  osVM.Descriptor
	libraryName string
	checksdPath string
}

func (v *sharedLibrarySuite) getSuiteOptions() []e2e.SuiteOption {
	agentConfig := `shared_library_check:
  enabled: "true"
  library_folder_path: ` + v.checksdPath

	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithIntegration("example.d", exampleCheckConfig),
				),
				ec2.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
			),
		),
	))

	return suiteOptions
}

func (v *sharedLibrarySuite) init() {
	// copy the lib with the right permissions
	sourceLibPath := path.Join(".", "files", v.libraryName)
	v.Env().RemoteHost.CopyFile(sourceLibPath, v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))

	// verify that the library has been successfully copied
	res, err := v.Env().RemoteHost.FileExists(v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))
	require.NoError(v.T(), err)
	require.True(v.T(), res)
}

func (v *sharedLibrarySuite) clean() {
	// remove lib after the test
	out := v.Env().RemoteHost.Remove(v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))
	// should not output anything, otherwise it's an error
	require.NoError(v.T(), out)

	// verify that the library has been successfully deleted
	res, err := v.Env().RemoteHost.FileExists(v.Env().RemoteHost.JoinPath(v.checksdPath, v.libraryName))
	require.NoError(v.T(), err)
	require.False(v.T(), res)
}

// Test the shared library code and check it returns the right metrics
func (v *sharedLibrarySuite) testCheckExecutionAndVerifyMetrics() {
	v.T().Log("Running Shared Library Check Example test")

	// execute the check and retrieve the metrics
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"example", "--json"}))
	data := checkutils.ParseJSONOutput(v.T(), []byte(check))

	// metric
	metrics := data[0].Aggregator.Metrics
	require.Len(v.T(), metrics, 1)

	metric := metrics[0]
	assert.Equal(v.T(), "hello.gauge", metric.Metric)
	assert.Equal(v.T(), 1.0, metric.Points[0][1])

	// service check
	serviceChecks := data[0].Aggregator.ServiceChecks
	require.Len(v.T(), serviceChecks, 1)

	serviceCheck := serviceChecks[0]
	assert.Equal(v.T(), "hello.service_check", serviceCheck.Name)
	assert.Equal(v.T(), 0, serviceCheck.Status)

	// event
	events := data[0].Aggregator.Events
	require.Len(v.T(), events, 1)

	event := events[0]
	assert.Equal(v.T(), "hello.event", event.Title)
	assert.Equal(v.T(), "hello.text", event.Text)
	assert.Equal(v.T(), "normal", event.Priority)
	assert.Equal(v.T(), "info", event.AlertType)
}

func (v *sharedLibrarySuite) testCheckExample() {
	v.init()
	v.testCheckExecutionAndVerifyMetrics()
	v.clean()
}
