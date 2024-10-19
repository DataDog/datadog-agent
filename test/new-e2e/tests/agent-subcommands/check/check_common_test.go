// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

import (
	_ "embed"
	"fmt"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type baseCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/hello.yaml
var customCheckYaml []byte

//go:embed fixtures/hello.py
var customCheckPython []byte

func (v *baseCheckSuite) TestCheckDisk() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"disk"}))

	assert.Contains(v.T(), check, `"metric": "system.disk.total"`)
	assert.Contains(v.T(), check, `"metric": "system.disk.used"`)
	assert.Contains(v.T(), check, `"metric": "system.disk.free"`)
}

func (v *baseCheckSuite) TestUnknownCheck() {
	_, err := v.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"unknown-check"}))
	assert.Error(v.T(), err)
	assert.Contains(v.T(), err.Error(), `Error: no valid check found`)
}

func (v *baseCheckSuite) TestCustomCheck() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello"}))
	assert.Contains(v.T(), check, `"metric": "hello.world"`)
	assert.Contains(v.T(), check, `"TAG_KEY:TAG_VALUE"`)
	assert.Contains(v.T(), check, `"type": "gauge"`)
}

func (v *baseCheckSuite) TestCheckRate() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--check-rate", "--json"}))
	data := parseCheckOutput(v.T(), []byte(check))

	metrics := data[0].Aggregator.Metrics

	assert.Equal(v.T(), len(metrics), 2)
	assert.Equal(v.T(), metrics[0].Metric, "hello.world")
	assert.Equal(v.T(), metrics[0].Points[0][1], 123)
	assert.Equal(v.T(), metrics[1].Metric, "hello.world")
	assert.Equal(v.T(), metrics[1].Points[0][1], 133)
}

func (v *baseCheckSuite) TestCheckTimes() {
	times := 10
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--check-times", fmt.Sprint(times), "--json"}))

	data := parseCheckOutput(v.T(), []byte(check))

	metrics := data[0].Aggregator.Metrics

	assert.Equal(v.T(), len(metrics), times)
	for idx := 0; idx < times; idx++ {
		assert.Equal(v.T(), metrics[idx].Points[0][1], 123+idx*10) // see fixtures/hello.py
	}
}
