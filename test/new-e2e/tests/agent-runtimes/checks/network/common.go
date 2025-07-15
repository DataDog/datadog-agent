// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checknetwork contains tests for the network check
package checknetwork

import (
	"slices"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	checkUtils "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-runtimes/checks/common"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type networkCheckSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor                  e2eos.Descriptor
	metricCompareDistance       int
	excludedFromValueComparison []string
}

func (v *networkCheckSuite) getSuiteOptions() []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
		),
	))

	return suiteOptions
}

func (v *networkCheckSuite) TestNetworkCheck() {
	testCases := []struct {
		name        string
		checkConfig string
		agentConfig string
		onlyLinux   bool
	}{
		{
			"default",
			`init_config:
instances:
  - collect_connection_state: false
`,
			``,
			false,
		},
		{
			"collect connection state",
			`init_config:
instances:
  - collect_connection_state: true
`,
			``,
			true, // this setting is not only for Linux but the windows python check is missing some metrics
		},
		// XXX: unfortunately the python version does not initialize all of the queue metrics so we cannot reliably compare them without them being "new"
		//{
		//	"collect connection queues",
		//	`init_config:
		//instances:
		//  - collect_connection_state: true
		//    collect_connection_queues: true
		//`,
		//	``,
		//	true,
		//},
	}

	for _, testCase := range testCases {
		if testCase.onlyLinux && v.descriptor.Family() != e2eos.LinuxFamily {
			continue
		}

		v.Run(testCase.name, func() {
			v.T().Log("run the network check using old version")
			pythonMetrics := v.runNetworkCheck(testCase.agentConfig, testCase.checkConfig, false)
			v.T().Log("run the network check using new version")
			goMetrics := v.runNetworkCheck(testCase.agentConfig, testCase.checkConfig, true)

			// assert the check output
			diff := gocmp.Diff(pythonMetrics, goMetrics,
				gocmp.Comparer(func(a, b check.Metric) bool {
					if !checkUtils.EqualMetrics(a, b) {
						return false
					}
					if slices.Contains(v.excludedFromValueComparison, a.Metric) {
						return true
					}
					aValue := a.Points[0][1]
					bValue := b.Points[0][1]
					return checkUtils.CompareValuesWithDistance(aValue, bValue, v.metricCompareDistance)
				}),
				gocmpopts.SortSlices(checkUtils.MetricPayloadCompare), // sort metrics
			)
			require.Empty(v.T(), diff)
		})
	}
}

func (v *networkCheckSuite) runNetworkCheck(agentConfig string, checkConfig string, useNewVersion bool) []check.Metric {
	if useNewVersion {
		agentConfig += "\nuse_networkv2_check: true"
		checkConfig += "\n    loader: core"
	}

	ctx := checkUtils.CheckContext{
		CheckName:    "network",
		OSDescriptor: v.descriptor,
		AgentConfig:  agentConfig,
		CheckConfig:  checkConfig,
		IsNewVersion: useNewVersion,
	}

	metrics := checkUtils.RunCheck(v.T(), v.Env(), ctx)

	return metrics
}
