// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains shared functions for running e2e tests against core check versions
package common

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

// CheckContext holds the configuration and data for a check instance run
type CheckContext struct {
	CheckName    string
	OSDescriptor e2eos.Descriptor
	AgentConfig  string
	CheckConfig  string
	IsNewVersion bool
}

// TODO:
// * format config yaml from struct

// EqualMetrics is a comparison function that compares struct fields between two metrics
func EqualMetrics(a, b check.Metric) bool {
	return a.Host == b.Host &&
		a.Interval == b.Interval &&
		a.Metric == b.Metric &&
		a.SourceTypeName == b.SourceTypeName &&
		a.Type == b.Type && gocmp.Equal(a.Tags, b.Tags, gocmpopts.SortSlices(cmp.Less[string]))
}

// CompareValuesWithRelativeMargin is a comparison functions that compares values to be within a relative range of each other
func CompareValuesWithRelativeMargin(a, b, p, fraction float64) bool {
	x := math.Round(a*p) / p
	y := math.Round(b*p) / p
	relMarg := fraction * math.Abs(x)
	return math.Abs(x-y) <= relMarg
}

// CompareValuesWithDistance is a comparison functions that compares values to be within an integer distance of each other
func CompareValuesWithDistance(a float64, b float64, distance int) bool {
	return math.Abs(a-b) <= float64(distance)
}

// MetricPayloadCompare is a comparison function that compares metric payloads
func MetricPayloadCompare(a, b check.Metric) int {
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

// RunCheck is the common utility function for running a CheckContext on a host environment and returns the metrics for comparison
func RunCheck(t *testing.T, env *environments.Host, ctxCheck CheckContext) []check.Metric {
	t.Helper()

	var checkVersionTag string
	if ctxCheck.IsNewVersion {
		checkVersionTag = fmt.Sprintf("%s_check_version:new", ctxCheck.CheckName)
	} else {
		checkVersionTag = fmt.Sprintf("%s_check_version:old", ctxCheck.CheckName)
	}
	checkConfig := fmt.Sprintf("%s\n    tags:\n      - %s", ctxCheck.CheckConfig, checkVersionTag)

	host := env.RemoteHost

	tmpFolder, err := host.GetTmpFolder()
	require.NoError(t, err)

	confFolder, err := host.GetAgentConfigFolder()
	require.NoError(t, err)

	// update agent configuration without restarting it, so that we can run both versions of the check
	// quickly one after the other, to minimize flakes in metric values
	extraConfigFilePath := host.JoinPath(tmpFolder, "datadog.yaml")
	_, err = host.WriteFile(extraConfigFilePath, []byte(ctxCheck.AgentConfig))
	require.NoError(t, err)
	// we need to write to a temp file and then copy due to permission issues
	tmpCheckConfigFile := host.JoinPath(tmpFolder, "check_config.yaml")
	_, err = host.WriteFile(tmpCheckConfigFile, []byte(checkConfig))
	require.NoError(t, err)

	checkConfigDir := fmt.Sprintf("%s.d", ctxCheck.CheckName)
	configFile := host.JoinPath(confFolder, "conf.d", checkConfigDir, "conf.yaml")
	if ctxCheck.OSDescriptor.Family() == e2eos.WindowsFamily {
		host.MustExecute(fmt.Sprintf("copy %s %s", tmpCheckConfigFile, configFile))
	} else {
		host.MustExecute(fmt.Sprintf("sudo -u dd-agent cp %s %s", tmpCheckConfigFile, configFile))
	}

	// run the check
	output := env.Agent.Client.Check(agentclient.WithArgs([]string{ctxCheck.CheckName, "--json", "--extracfgpath", extraConfigFilePath}))
	data := check.ParseJSONOutput(t, []byte(output))

	require.Len(t, data, 1)
	metrics := data[0].Aggregator.Metrics
	for i := range metrics {
		// remove the disk_check_version tag
		tagLen := len(metrics[i].Tags)
		metrics[i].Tags = slices.DeleteFunc(metrics[i].Tags, func(tag string) bool {
			return tag == checkVersionTag
		})
		removedElements := tagLen - len(metrics[i].Tags)
		if !assert.Equalf(t, 1, removedElements, "expected tag %s once in metric %s", checkVersionTag, metrics[i].Metric) {
			t.Logf("metric: %+v", metrics[i])
		}
	}

	return metrics
}
