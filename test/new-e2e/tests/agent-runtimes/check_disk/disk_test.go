// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checkdisk contains tests for the disk check
package checkdisk

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

const metricCompareTolerance = 0.01

func TestLinuxDiskSuite(t *testing.T) {
	agentOptions := []agentparams.Option{}
	suiteOptions := []e2e.SuiteOption{}
	//TODO: remove once the test is ready for CI
	if os.Getenv("CI_PIPELINE_ID") == "" {
		// if running locally, use the hardcoded pipeline id
		agentOptions = append(agentOptions,
			// update pipeline id when you push changes to the disk check or the agent itself
			agentparams.WithPipeline("61451328"),
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
			ElementsMatch(v.T(), pythonMetrics, goMetrics, metricPayloadCompare)
			v.T().Logf("the check emitted %d metrics", len(pythonMetrics))
		})
	}
}

type metricPayload struct {
	name           string
	tags           []string
	points         []float64
	ty             int32
	unit           string
	SourceTypeName string
	interval       int64
	originProduct  uint32
	originCategory uint32
	originService  uint32
}

func metricPayloadCompare(a, b metricPayload) bool {
	return a.name == b.name &&
		slices.Equal(a.tags, b.tags) &&
		slices.EqualFunc(a.points, b.points, func(a, b float64) bool {
			if a == 0 {
				return b == 0
			}
			return math.Abs(a-b)/a <= metricCompareTolerance
		}) &&
		a.ty == b.ty &&
		a.unit == b.unit &&
		a.SourceTypeName == b.SourceTypeName &&
		a.interval == b.interval &&
		a.originProduct == b.originProduct &&
		a.originCategory == b.originCategory &&
		a.originService == b.originService
}

func (v *baseCheckSuite) runDiskCheck(agentConfig string, checkConfig string, useNewVersion bool) []metricPayload {
	if useNewVersion {
		agentConfig += "\nuse_diskv2_check: true"
		checkConfig += "\n  loader: core"
	}

	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithFile("/etc/datadog-agent/conf.d/conf.yaml", checkConfig, true),
		),
	))

	// flush fakeintake payloads
	err := v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(v.T(), err)

	// run the check
	v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"disk"}))

	// wait for the metrics to be received by the fakeintake
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("system.disk.total")
		require.NoError(c, err)
		require.NotEmpty(c, metrics)
	}, time.Second*10, time.Second*1)

	// get the metric payloads for the disk check
	metrics, err := v.Env().FakeIntake.Client().GetMetricNames()
	require.NoError(v.T(), err)
	var diskMetricPayloads []metricPayload
	for _, metric := range metrics {
		if !strings.HasPrefix(metric, "system.disk.") &&
			!strings.HasPrefix(metric, "system.fs.inodes.") {
			continue
		}

		payloads, err := v.Env().FakeIntake.Client().FilterMetrics(metric)
		require.NoError(v.T(), err)
		for _, payload := range payloads {
			points := make([]float64, len(payload.Points))
			for i, point := range payload.Points {
				points[i] = point.Value
			}

			diskMetricPayloads = append(diskMetricPayloads, metricPayload{
				name:           payload.Metric,
				tags:           payload.Tags,
				points:         points,
				ty:             int32(payload.Type),
				unit:           payload.Unit,
				SourceTypeName: payload.SourceTypeName,
				interval:       payload.Interval,
				originProduct:  payload.GetMetadata().Origin.GetOriginProduct(),
				originCategory: payload.GetMetadata().Origin.GetOriginCategory(),
				originService:  payload.GetMetadata().Origin.GetOriginService(),
			})
		}
	}

	return diskMetricPayloads
}
