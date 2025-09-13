// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector contains tests for the collector
package collector

import (
	_ "embed"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	osVM "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

const pythonCheck = `
from datadog_checks.base import AgentCheck

class HTTPCheck(AgentCheck):
	def check(self, instance):
		checkId = instance.get('id')
		print(f"Running check {checkId}")
`

const pythonCheckConfig = `
init_config:
instances:
  - id: 1
`

type baseCollectorSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	checksdPath string
}

func (v *baseCollectorSuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithFile(v.checksdPath, pythonCheck, true),
				agentparams.WithIntegration("my_check.d", pythonCheckConfig),
			),
			awshost.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
		),
	))

	return suiteOptions
}

func (v *baseCollectorSuite) TestCollector() {
	v.Env().Docker.Client.ListContainers()
}
