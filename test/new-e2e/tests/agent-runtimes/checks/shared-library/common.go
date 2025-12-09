// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedlibrary contains tests for the shared library checks
package sharedlibrary

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	osVM "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	checkutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed files/conf.yaml
var exampleCheckYaml string

type sharedLibrarySuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor  osVM.Descriptor
	libraryName string
	checksdPath string
}

func (v *sharedLibrarySuite) getSuiteOptions() []e2e.SuiteOption {
	config := `shared_library_check.enabled: true
shared_library_check.library_folder_path: ` + v.checksdPath

	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithIntegration("example.d", exampleCheckYaml),
					agentparams.WithAgentConfig(config),
				),
				ec2.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
			),
		),
	))

	return suiteOptions
}

// Test the shared library code and check it returns the right metrics
func (v *sharedLibrarySuite) testCheckExecutionAndVerifyMetrics() {
	v.T().Log("Running Shared Library Check Example test")

	// execute the check and retrieve the metrics
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"example", "--json"}))
	data := checkutils.ParseJSONOutput(v.T(), []byte(check))
	metrics := data[0].Aggregator.Metrics

	// only one metric should have been emitted
	assert.Equal(v.T(), len(metrics), 1)

	// check metric info
	metric := metrics[0]
	assert.Equal(v.T(), "hello.gauge", metric.Metric)
	assert.Equal(v.T(), metric.Points[0][1], 1.0)
}
