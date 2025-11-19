// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for the rtloader
package rtloader

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	osVM "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	checkutils "github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed python-check/multi_pid_check.py
var multiPidCheckPy string

//go:embed python-check/multi_pid_check.yaml
var multiPidCheckYaml string

type baseMultiProcessingLibSuite struct {
	e2e.BaseSuite[environments.Host]

	checksdPath string
}

func (v *baseMultiProcessingLibSuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithIntegration("multi_pid_check.d", multiPidCheckYaml),
					agentparams.WithFile(v.checksdPath, multiPidCheckPy, true),
				),
				ec2.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
			),
		),
	))

	return suiteOptions
}

func (v *baseMultiProcessingLibSuite) TestMultiProcessingLib() {
	v.T().Log("Running MultiProcessingLib test")

	// Fetch the check status and metrics in JSON format
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"multi_pid_check", "--json"}))
	data := checkutils.ParseJSONOutput(v.T(), []byte(check))
	metrics := data[0].Aggregator.Metrics
	assert.GreaterOrEqual(v.T(), len(metrics), 3)

	pidMap := make(map[float64]bool)
	for _, metric := range metrics {
		assert.Equal(v.T(), "multi_pid_check.process.pid", metric.Metric)
		pid := metric.Points[0][1]
		assert.False(v.T(), pidMap[pid], "PID %f is not unique", pid)
		pidMap[pid] = true
	}
}
