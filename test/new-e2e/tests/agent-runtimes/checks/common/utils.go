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

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/testcommon/check"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type CheckContext struct {
	checkName    string
	agentConfig  string
	checkConfig  string
	isNewVersion bool
}

// TODO:
// * format config yaml from struct

func EqualMetrics(a, b check.Metric) bool {
	return a.Host == b.Host &&
		a.Interval == b.Interval &&
		a.Metric == b.Metric &&
		a.SourceTypeName == b.SourceTypeName &&
		a.Type == b.Type && gocmp.Equal(a.Tags, b.Tags, gocmpopts.SortSlices(cmp.Less[string]))
}

func CompareValuesWithRelativeMargin(a, b, p, fraction float64) bool {
	x := math.Round(a*p) / p
	y := math.Round(b*p) / p
	relMarg := fraction * math.Abs(x)
	return math.Abs(x-y) <= relMarg
}

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

func RunCheck(v e2e.Suite, ctxCheck CheckContext) []check.Metric {
	v.T().Helper()

	var checkVersionTag string
	if ctxCheck.isNewVersion {
		checkVersionTag = fmt.Sprintf("%s_check_version:new", ctxCheck.checkName)
	} else {
		checkVersionTag = fmt.Sprintf("%s_check_version:old", ctxCheck.checkName)
	}
	checkConfig = fmt.Sprintf("%s\n    tags:\n      - %s", ctxCheck.checkConfig, checkVersionTag)
	host := v.Env().RemoteHost

	tmpFolder, err := host.GetTmpFolder()
	require.NoError(v.T(), err)

	confFolder, err := host.GetAgentConfigFolder()
	require.NoError(v.T(), err)

	// update agent configuration without restarting it, so that we can run both versions of the check
	// quickly one after the other, to minimize flakes in metric values
	extraConfigFilePath := host.JoinPath(tmpFolder, "datadog.yaml")
	_, err = host.WriteFile(extraConfigFilePath, []byte(ctxCheck.agentConfig))
	require.NoError(v.T(), err)
	// we need to write to a temp file and then copy due to permission issues
	tmpCheckConfigFile := host.JoinPath(tmpFolder, "check_config.yaml")
	_, err = host.WriteFile(tmpCheckConfigFile, []byte(checkConfig))
	require.NoError(v.T(), err)

	checkConfigDir := fmt.Sprintf("%s.d", ctxCheck.checkName)
	configFile := host.JoinPath(confFolder, "conf.d", checkConfigDir, "conf.yaml")
	if v.descriptor.Family() == e2eos.WindowsFamily {
		host.MustExecute(fmt.Sprintf("copy %s %s", tmpCheckConfigFile, configFile))
	} else {
		host.MustExecute(fmt.Sprintf("sudo -u dd-agent cp %s %s", tmpCheckConfigFile, configFile))
	}

	// run the check
	output := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{ctxCheck.checkName, "--json", "--extracfgpath", extraConfigFilePath}))
	data := check.ParseJSONOutput(v.T(), []byte(output))

	require.Len(v.T(), data, 1)
	metrics := data[0].Aggregator.Metrics
	for i := range metrics {
		// remove the disk_check_version tag
		tagLen := len(metrics[i].Tags)
		metrics[i].Tags = slices.DeleteFunc(metrics[i].Tags, func(tag string) bool {
			return tag == checkVersionTag
		})
		removedElements := tagLen - len(metrics[i].Tags)
		if !assert.Equalf(v.T(), 1, removedElements, "expected tag %s once in metric %s", diskCheckVersion, metrics[i].Metric) {
			v.T().Logf("metric: %+v", metrics[i])
		}
	}

	return metrics
}
