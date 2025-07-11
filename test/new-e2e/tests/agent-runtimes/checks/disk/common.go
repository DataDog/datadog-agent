// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"math"
	"slices"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	checkUtils "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-runtimes/checks/common"
)

type diskCheckSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor                  e2eos.Descriptor
	metricCompareFraction       float64
	metricCompareDecimals       int
	excludedFromValueComparison []string
}

func (v *diskCheckSuite) getSuiteOptions() []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
		),
	))

	return suiteOptions
}

// printDiskUsage prints the disk usage for the current environment
// used to debug the disk check getting different values on some runs
func (v *diskCheckSuite) printDiskUsage() {
	if v.descriptor.Family() != e2eos.WindowsFamily {
		// only the windows version is flaky so no need to handle linux
		return
	}

	diskUsage, err := v.Env().RemoteHost.Execute("Get-PSDrive -Name C")
	if err != nil {
		v.T().Logf("error getting disk usage: %s", err)
		return
	}
	v.T().Logf("disk usage: %s", diskUsage)
}

func (v *diskCheckSuite) TestCheckDisk() {
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
  - use_mount: false
`,
			``,
			false,
		},
		{
			"all partitions",
			`init_config:
instances:
  - use_mount: true
    all_partitions: true
`,
			``,
			false,
		},
		{
			"tag by filesystem",
			`init_config:
instances:
  - use_mount: false
    tag_by_filesystem: true
`,
			``,
			false,
		},
		{
			"do not tag by label",
			`init_config:
instances:
  - use_mount: false
    tag_by_label: false
`,
			``,
			true,
		},
		{
			"use lsblk",
			`init_config:
instances:
  - use_mount: false
    use_lsblk: true
`,
			``,
			true,
		},
	}
	p := math.Pow10(v.metricCompareDecimals)
	for _, testCase := range testCases {
		if testCase.onlyLinux && v.descriptor.Family() != e2eos.LinuxFamily {
			continue
		}

		v.Run(testCase.name, func() {
			v.T().Log("run the disk check using old version")
			v.printDiskUsage()
			pythonMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, false)
			v.T().Log("run the disk check using new version")
			v.printDiskUsage()
			goMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, true)
			v.printDiskUsage()

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
					// system.disk.total metric is expected to be strictly equal between both checks
					if a.Metric == "system.disk.total" {
						return aValue == bValue
					}
					return checkUtils.CompareValuesWithRelativeMargin(aValue, bValue, p, v.metricCompareFraction)
				}),
				gocmpopts.SortSlices(checkUtils.MetricPayloadCompare), // sort metrics
			)
			require.Empty(v.T(), diff)
		})
	}
}

func (v *diskCheckSuite) runDiskCheck(agentConfig string, checkConfig string, useNewVersion bool) []check.Metric {
	if useNewVersion {
		agentConfig += "\nuse_diskv2_check: true"
		checkConfig += "\n    loader: core"
	}

	ctx := checkUtils.CheckContext{
		CheckName:    "disk",
		OSDescriptor: v.descriptor,
		AgentConfig:  agentConfig,
		CheckConfig:  checkConfig,
		IsNewVersion: useNewVersion,
	}

	metrics := checkUtils.RunCheck(v.T(), v.Env(), ctx)

	return metrics
}
