// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"fmt"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

const metricCompareTolerance = 0.02

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
			elementsMatch(v.T(), pythonMetrics, goMetrics, metricPayloadCompare)
			v.T().Logf("the check emitted %d metrics", len(pythonMetrics))
		})
	}
}

func metricPayloadCompare(a, b check.Metric) bool {
	return a.Host == b.Host &&
		a.Interval == b.Interval &&
		a.Metric == b.Metric &&
		slices.EqualFunc(a.Points, b.Points, func(a, b []float64) bool {
			return slices.EqualFunc(a, b, func(a, b float64) bool {
				if a == 0 {
					return b == 0
				}
				return math.Abs(a-b)/a <= metricCompareTolerance
			})
		}) &&
		a.SourceTypeName == b.SourceTypeName &&
		a.Type == b.Type &&
		compareTags(a.Tags, b.Tags)
}

// compare the tags in both slices
// not sure if there can be duplicates in the tags, but let's assume so
// this is inefficient, but good enough for this test
func compareTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	tagsA := make(map[string]int, len(a))
	for _, tag := range a {
		tagsA[tag]++
	}
	for _, tag := range b {
		tagsA[tag]--
	}
	for _, count := range tagsA {
		if count != 0 {
			return false
		}
	}
	return true
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
	data := check.ParseJSONOutput(v.T(), []byte(output))

	require.Len(v.T(), data, 1)
	metrics := data[0].Aggregator.Metrics
	for i := range metrics {
		// remove the disk_check_version tag
		metrics[i].Tags = slices.DeleteFunc(metrics[i].Tags, func(tag string) bool {
			return tag == diskCheckVersionTag
		})
	}

	return metrics
}
