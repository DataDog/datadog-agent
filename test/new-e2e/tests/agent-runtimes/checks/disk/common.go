// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"math"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/checks/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
)

type diskCheckSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor            e2eos.Descriptor
	metricCompareFraction float64
	metricCompareDecimals int
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

func (v *diskCheckSuite) TestCheckDisk() {
	testCases := []struct {
		name        string
		checkConfig string
		agentConfig string
	}{
		{
			"default",
			`init_config:
instances:
  - use_mount: false
`,
			``,
		},
	}
	p := math.Pow10(v.metricCompareDecimals)
	for _, testCase := range testCases {
		v.Run(testCase.name, func() {
			v.T().Log("run the disk check using old version")
			pythonMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, false)
			v.T().Log("run the disk check using new version")
			goMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, true)

			// assert the check output
			diff := gocmp.Diff(pythonMetrics, goMetrics,
				gocmp.Comparer(func(a, b check.Metric) bool {
					if !common.EqualMetrics(a, b) {
						return false
					}
					aValue := a.Points[0][1]
					bValue := b.Points[0][1]
					// system.disk.total metric is expected to be strictly equal between both checks
					if a.Metric == "system.disk.total" {
						return aValue == bValue
					}
					return common.CompareValuesWithRelativeMargin(aValue, bValue, p, v.metricCompareFraction)
				}),
				gocmpopts.SortSlices(common.MetricPayloadCompare), // sort metrics
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

	ctx := common.CheckContext{
		checkName:    "disk",
		agentConfig:  agentConfig,
		checkConfig:  checkConfig,
		isNewVersion: useNewVersion,
	}

	metrics := common.RunCheck(v, ctx)

	return metrics
}
