// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseCheckSuite struct {
	e2e.BaseSuite[environments.Host]
	descriptor            e2eos.Descriptor
	metricCompareFraction float64
	metricCompareDecimals int
}

func (v *baseCheckSuite) getSuiteOptions() []e2e.SuiteOption {
	suiteOptions := []e2e.SuiteOption{}
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
		),
	))

	return suiteOptions
}

// metrics that remain constant
var constantMetricsSet = map[string]struct{}{
	"system.disk.total": {},
}

func (v *baseCheckSuite) TestCheckDisk() {
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
	p := math.Pow(10, float64(v.metricCompareDecimals))
	for _, testCase := range testCases {
		v.Run(testCase.name, func() {
			v.T().Log("run the disk check using old version")
			pythonMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, false)
			v.T().Log("run the disk check using new version")
			goMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, true)

			// assert the check output
			diff := gocmp.Diff(pythonMetrics, goMetrics,
				gocmp.Comparer(func(a, b check.Metric) bool {
					if a.Host != b.Host ||
						a.Interval != b.Interval ||
						a.Metric != b.Metric ||
						a.SourceTypeName != b.SourceTypeName ||
						a.Type != b.Type {
						return false
					}
					if !gocmp.Equal(a.Tags, b.Tags, gocmpopts.SortSlices(cmp.Less[string])) {
						return false
					}
					aValue := a.Points[0][1]
					bValue := b.Points[0][1]
					if _, ok := constantMetricsSet[a.Metric]; ok {
						return aValue == bValue
					}
					return compareValuesWithRelativeMargin(aValue, bValue, p, v.metricCompareFraction)
				}),
				gocmpopts.SortSlices(metricPayloadCompare), // sort metrics
			)
			require.Empty(v.T(), diff)
		})
	}
}

func compareValuesWithRelativeMargin(a, b, p, fraction float64) bool {
	x := math.Round(a*p) / p
	y := math.Round(b*p) / p
	relMarg := fraction * math.Min(math.Abs(x), math.Abs(y))
	return math.Abs(x-y) <= relMarg
}

func metricPayloadCompare(a, b check.Metric) int {
	return cmp.Or(
		cmp.Compare(a.Host, b.Host),
		cmp.Compare(a.Metric, b.Metric),
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.SourceTypeName, b.SourceTypeName),
		cmp.Compare(a.Interval, b.Interval),
		slices.Compare(a.Tags, b.Tags),
		slices.CompareFunc(a.Points, b.Points, func(a, b []float64) int {
			return slices.Compare(a, b)
		}),
	)
}

func (v *baseCheckSuite) runDiskCheck(agentConfig string, checkConfig string, useNewVersion bool) []check.Metric {
	v.T().Helper()

	diskCheckVersion := "old"
	if useNewVersion {
		agentConfig += "\nuse_diskv2_check: true"
		checkConfig += "\n    loader: core"
		diskCheckVersion = "new"
	}
	diskCheckVersionTag := fmt.Sprintf("disk_check_version:%s", diskCheckVersion)
	checkConfig += fmt.Sprintf("\n    tags:\n      - %s", diskCheckVersionTag)

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(v.descriptor)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("disk.d", checkConfig))),
	)

	// run the check
	output := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"disk", "--json"}))
	data := check.ParseJSONOutput(v.T(), []byte(output))

	require.Len(v.T(), data, 1)
	metrics := data[0].Aggregator.Metrics
	for i := range metrics {
		// remove the disk_check_version tag
		tagLen := len(metrics[i].Tags)
		metrics[i].Tags = slices.DeleteFunc(metrics[i].Tags, func(tag string) bool {
			return tag == diskCheckVersionTag
		})
		removedElements := tagLen - len(metrics[i].Tags)
		if !assert.Equalf(v.T(), 1, removedElements, "expected tag %s once in metric %s", diskCheckVersion, metrics[i].Metric) {
			v.T().Logf("metric: %+v", metrics[i])
		}
	}

	return metrics
}
