// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type linuxCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed fixtures/hello.yaml
var customCheckYaml []byte

//go:embed fixtures/hello.py
var customCheckPython []byte

func TestLinuxCheckSuite(t *testing.T) {
	e2e.Run(t, &linuxCheckSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(
		agentparams.WithFile("/etc/datadog-agent/conf.d/hello.yaml", string(customCheckYaml), true),
		agentparams.WithFile("/etc/datadog-agent/checks.d/hello.py", string(customCheckPython), true),
	))), e2e.WithDevMode())
}

func (v *linuxCheckSuite) TestCheckDisk() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"disk"}))

	assert.Contains(v.T(), check, `"metric": "system.disk.total"`)
	assert.Contains(v.T(), check, `"metric": "system.disk.used"`)
	assert.Contains(v.T(), check, `"metric": "system.disk.free"`)
}

func (v *linuxCheckSuite) TestUnknownCheck() {
	_, err := v.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"unknown-check"}))
	assert.Error(v.T(), err)
	assert.Contains(v.T(), err.Error(), `Error: no valid check found`)
}

func (v *linuxCheckSuite) TestCustomCheck() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello"}))
	assert.Contains(v.T(), check, `"metric": "hello.world"`)
	assert.Contains(v.T(), check, `"TAG_KEY:TAG_VALUE"`)
	assert.Contains(v.T(), check, `"type": "gauge"`)
}

func (v *linuxCheckSuite) TestCheckRate() {
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--check-rate", "--json"}))
	data := parseCheckOutput([]byte(check))
	require.NotNil(v.T(), data)

	metrics := data[0].Aggregator.Metrics

	assert.Equal(v.T(), len(metrics), 2)
	assert.Equal(v.T(), metrics[0].Metric, "hello.world")
	assert.Equal(v.T(), metrics[0].Points[0][1], 123)
	assert.Equal(v.T(), metrics[1].Metric, "hello.world")
	assert.Equal(v.T(), metrics[1].Points[0][1], 133)
}

func (v *linuxCheckSuite) TestCheckTimes() {
	times := 10
	check := v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--check-times", fmt.Sprint(times), "--json"}))

	data := parseCheckOutput([]byte(check))
	require.NotNil(v.T(), data)

	metrics := data[0].Aggregator.Metrics

	assert.Equal(v.T(), len(metrics), times)
	for idx := 0; idx < times; idx++ {
		assert.Equal(v.T(), metrics[idx].Points[0][1], 123+idx*10) // see fixtures/hello.py
	}
}

func (v *linuxCheckSuite) TestCheckFlare() {
	v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--flare"}))
	files := v.Env().RemoteHost.MustExecute("sudo ls /var/log/datadog/checks")
	assert.Contains(v.T(), files, "check_hello")
}
