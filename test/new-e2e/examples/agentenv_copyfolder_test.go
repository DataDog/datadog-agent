// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/require"
)

type agentSuiteEx6 struct {
	e2e.Suite[environments.Host]
}

func TestAgentSuiteEx6(t *testing.T) {
	e2e.Run(t, &agentSuiteEx6{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *agentSuiteEx6) TestCopy() {
	testFolder := path.Join(os.TempDir(), "test-folder")
	err := createFoldersEx6(testFolder)
	require.NoError(v.T(), err)

	file, err := os.ReadFile(path.Join(testFolder, "hosts"))
	require.NoError(v.T(), err)
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithFile("/etc/hosts", string(file), true))))

	v.Env().RemoteHost.CopyFolder(path.Join(testFolder), "test")
	v.Env().RemoteHost.CopyFile(path.Join(testFolder, "file-0"), "copied-file")

	output0 := v.Env().RemoteHost.MustExecute("cat test/file-0")
	output1 := v.Env().RemoteHost.MustExecute("cat test/folder-1/file-1")
	output2 := v.Env().RemoteHost.MustExecute("cat copied-file")
	output3 := v.Env().RemoteHost.MustExecute("cat /etc/hosts")

	require.Equal(v.T(), "This is a test file 0", output0)
	require.Equal(v.T(), "This is a test file 1", output1)
	require.Equal(v.T(), "This is a test file 0", output2)
	require.Equal(v.T(), "127.0.0.1			localhost\n127.0.0.1           agentvm\n", output3)
}

func createFoldersEx6(folder string) error {
	err := os.MkdirAll(folder, 0o755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(path.Join(folder, "folder-1"), 0o755)
	if err != nil {
		return err
	}

	err = os.WriteFile(path.Join(folder, "file-0"), []byte("This is a test file 0"), 0o655)
	if err != nil {
		return err
	}
	err = os.WriteFile(path.Join(folder, "folder-1", "file-1"), []byte("This is a test file 1"), 0o655)
	if err != nil {
		return err
	}

	err = os.WriteFile(path.Join(folder, "hosts"), []byte("127.0.0.1			localhost\n127.0.0.1           agentvm\n"), 0o655)
	if err != nil {
		return err
	}
	return nil
}
