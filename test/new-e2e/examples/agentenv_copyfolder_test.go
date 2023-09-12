// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"path"
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

	e2e.Run(t, &agentSuiteEx6{}, e2e.AgentStackDef([]ec2params.Option{ec2params.WithOS(ec2os.UbuntuOS)}))
}

func (v *agentSuiteEx6) TestCopy() {

	testFolder := path.Join(os.TempDir(), "test-folder")
	err := createFoldersEx6(testFolder)
	require.NoError(v.T(), err)

	file, err := os.ReadFile(path.Join(testFolder, "hosts"))
	require.NoError(v.T(), err)
	v.UpdateEnv(e2e.AgentStackDef([]ec2params.Option{ec2params.WithOS(ec2os.UbuntuOS)}, agentparams.WithFile("/etc/hosts", string(file), true)))

	v.Env().VM.CopyFolder(path.Join(testFolder), "test")
	v.Env().VM.CopyFile(path.Join(testFolder, "file-0"), "copied-file")

	output0 := v.Env().VM.Execute("cat test/file-0")
	output1 := v.Env().VM.Execute("cat test/folder-1/file-1")
	output2 := v.Env().VM.Execute("cat copied-file")
	output3 := v.Env().VM.Execute("cat /etc/hosts")

	require.Equal(v.T(), "This is a test file 0", output0)
	require.Equal(v.T(), "This is a test file 1", output1)
	require.Equal(v.T(), "This is a test file 0", output2)
	require.Equal(v.T(), "127.0.0.1			localhost\n127.0.0.1           agentvm\n", output3)
}

func createFoldersEx6(folder string) error {

	err := os.MkdirAll(folder, 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(path.Join(folder, "folder-1"), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(path.Join(folder, "file-0"), []byte("This is a test file 0"), 0655)
	if err != nil {
		return err
	}
	err = os.WriteFile(path.Join(folder, "folder-1", "file-1"), []byte("This is a test file 1"), 0655)
	if err != nil {
		return err
	}

	err = os.WriteFile(path.Join(folder, "hosts"), []byte("127.0.0.1			localhost\n127.0.0.1           agentvm\n"), 0655)
	if err != nil {
		return err
	}
	return nil
}
