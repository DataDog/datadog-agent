// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedlibrary contains tests for the shared library checks
package sharedlibrary

import (
	_ "embed"
	"fmt"
	"os"
	"path"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	osVM "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	checkutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed files/minimal_conf.yaml
var checkMinimalConfig string

const libraryPrefix = "libdatadog-agent-"

type sharedLibrarySuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor  osVM.Descriptor
	checksdPath string
}

func (v *sharedLibrarySuite) newProvisionerWithAgentOptions(agentOptions ...agentparams.Option) provisioners.Provisioner {
	agentConfig := `shared_library_check:
  enabled: true
  library_folder_path: ` + v.checksdPath

	// default option - enable shared library checks
	allAgentOptions := []agentparams.Option{agentparams.WithAgentConfig(agentConfig)}

	// additional options
	allAgentOptions = append(allAgentOptions, agentOptions...)

	return awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
			ec2.WithAgentOptions(allAgentOptions...),
		),
	)
}

func (v *sharedLibrarySuite) getSuiteOptions() e2e.SuiteOption {
	return e2e.WithProvisioner(v.newProvisionerWithAgentOptions())
}

func (v *sharedLibrarySuite) resolveSharedLibraryFileName(name string) (string, error) {
	var libraryExtension string

	// get the library extension based on the OS running in the remote VM
	switch v.descriptor.Flavor.Type() {
	case osVM.LinuxFamily:
		libraryExtension = "so"
	case osVM.WindowsFamily:
		libraryExtension = "dll"
	case osVM.MacOSFamily:
		libraryExtension = "dylib"
	default:
		return "", fmt.Errorf("unknown OS family: %v", v.descriptor.Flavor.Type())
	}

	return libraryPrefix + name + "." + libraryExtension, nil
}

func (v *sharedLibrarySuite) updateEnvWithCheckConfigAndSharedLibrary(name string, config string, permissions option.Option[perms.FilePermissions]) {
	// get the content of the local shared library
	libraryName, err := v.resolveSharedLibraryFileName(name)
	require.NoError(v.T(), err)

	libraryContent, err := os.ReadFile(path.Join("files", libraryName))
	require.NoError(v.T(), err)

	// option to copy the shared library to the remote host
	libraryPath := path.Join(v.checksdPath, libraryName)
	fileOption := agentparams.WithFileWithPermissions(libraryPath, string(libraryContent), true, permissions)

	// option to configure the check in the remote Agent
	integrationOption := agentparams.WithIntegration(name+".d", config)

	// update the remote agent with all options
	agentOptions := []agentparams.Option{fileOption, integrationOption}

	v.UpdateEnv(v.newProvisionerWithAgentOptions(agentOptions...))
}

// Test the shared library code and check it returns the right metrics
func (v *sharedLibrarySuite) testCheckExampleExecutionAndMetrics() {
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

// Test the shared library code and check it returns the right metrics
func (v *sharedLibrarySuite) testCheckWithoutRunSymbolExecutionError() {
	_, err := v.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"no-run-symbol", "--json"}))

	// expect to have an error about the `Run` symbol
	assert.ErrorContains(v.T(), err, "can't find 'Run' symbol")
}
