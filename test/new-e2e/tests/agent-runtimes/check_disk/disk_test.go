// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-subcommands/check"
)

type baseCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

// a relative diff considered acceptable when comparing metrics
const metricCompareFraction = 0.02

// an absolute diff considered acceptable when comparing metrics
const metricCompareMargin = 0.001

func TestLinuxDiskSuite(t *testing.T) {
	agentOptions := []agentparams.Option{}
	suiteOptions := []e2e.SuiteOption{}
	//TODO: remove once the test is ready for CI
	if os.Getenv("CI_PIPELINE_ID") == "" {
		// if running locally, use the hardcoded pipeline id
		agentOptions = append(agentOptions,
			// update pipeline id when you push changes to the disk check or the agent itself
			agentparams.WithPipeline("61736511"),
		)
		// helpful for local runs
		suiteOptions = append(suiteOptions, e2e.WithDevMode())
		suiteOptions = append(suiteOptions, e2e.WithStackName("disk-check-test"))
	}

	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentOptions...,
			),
		),
	))

	t.Parallel()
	e2e.Run(t, &baseCheckSuite{},
		suiteOptions...,
	)
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

	for _, testCase := range testCases {
		v.Run(testCase.name, func() {
			v.T().Log("run the disk check using old version")
			pythonMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, false)
			v.T().Log("run the disk check using new version")
			goMetrics := v.runDiskCheck(testCase.agentConfig, testCase.checkConfig, true)

			// assert the check output
			diff := gocmp.Diff(pythonMetrics, goMetrics,
				gocmpopts.EquateApprox(metricCompareFraction, metricCompareMargin),
				gocmpopts.SortSlices(cmp.Less[string]),     // sort tags
				gocmpopts.SortSlices(metricPayloadCompare), // sort metrics
			)
			require.Empty(v.T(), diff)
		})
	}
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
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("disk.d", checkConfig),
		),
	))

	// run the check
	output := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"disk", "--json"}))
	data := check.ParseCheckOutput(v.T(), []byte(output))

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
