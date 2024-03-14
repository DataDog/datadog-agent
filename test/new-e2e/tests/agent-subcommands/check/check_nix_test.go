// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains helpers and e2e tests of the check command
package check

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type linuxCheckSuite struct {
	baseCheckSuite
}

//go:embed fixtures/hello.yaml
var customCheckYaml []byte

//go:embed fixtures/hello.py
var customCheckPython []byte

func TestLinuxCheckSuite(t *testing.T) {
	e2e.Run(t, &linuxCheckSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(
		agentparams.WithFile("/etc/datadog-agent/conf.d/hello.yaml", string(customCheckYaml), true),
		agentparams.WithFile("/etc/datadog-agent/checks.d/hello.py", string(customCheckPython), true),
	))))
}

func (v *linuxCheckSuite) TestCheckFlare() {
	v.Env().Agent.Client.Check(agentclient.WithArgs([]string{"hello", "--flare"}))
	files := v.Env().RemoteHost.MustExecute("sudo ls /var/log/datadog/checks")
	assert.Contains(v.T(), files, "check_hello")
}
