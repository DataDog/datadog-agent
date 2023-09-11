// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

type agentSuiteEx6 struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentSuiteEx6(t *testing.T) {
	file, err := os.ReadFile("test-folder/file-0")
	require.NoError(t, err)
	e2e.Run(t, &agentSuiteEx6{}, e2e.AgentStackDef([]ec2params.Option{ec2params.WithOS(ec2os.UbuntuOS)}, agentparams.WithFile("/home/ubuntu/testfilewithfile", string(file), false)))
}

func (v *agentSuiteEx6) TestCopy() {

	v.Env().VM.CopyFolder("test-folder", "test")
	v.Env().VM.CopyFile("test-folder/file-0", "copied-file")

	output0 := v.Env().VM.Execute("cat test/file-0")
	output1 := v.Env().VM.Execute("cat test/folder-1/file-1")
	output2 := v.Env().VM.Execute("cat copied-file")
	output3 := v.Env().VM.Execute("cat testfilewithfile")

	require.Equal(v.T(), "This is a test file 0\n", output0)
	require.Equal(v.T(), "This is a test file 1\n", output1)
	require.Equal(v.T(), "This is a test file 0\n", output2)
	require.Equal(v.T(), "This is a test file 0\n", output3)
}
