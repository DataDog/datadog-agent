// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type fileManagerSuiteEx7 struct {
	e2e.BaseSuite[environments.Host]
}

func customProvisionerFileManager(localFolderPath string, remoteFolderPath string) provisioners.PulumiEnvRunFunc[environments.Host] {
	return func(ctx *pulumi.Context, env *environments.Host) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		vm, err := ec2.NewVM(awsEnv, "vm", ec2.WithOS(osComp.UbuntuDefault))
		if err != nil {
			return err
		}
		vm.Export(ctx, &env.RemoteHost.HostOutput)

		vm.OS.FileManager().CopyAbsoluteFolder(localFolderPath, remoteFolderPath)

		// To partially re-use an existing environment, you need to make sure that unused components are nil
		// Otherwise create your own environment.
		env.Agent = nil
		env.FakeIntake = nil

		return nil
	}
}

func TestFileManagerSuiteEx7(t *testing.T) {
	testFolder := path.Join(os.TempDir(), "test-folder")
	createFolders(testFolder)
	e2e.Run(t, &fileManagerSuiteEx7{}, e2e.WithPulumiProvisioner(customProvisionerFileManager(testFolder, "/home/ubuntu/test"), nil))
}

func (v *fileManagerSuiteEx7) TestCopy() {
	output0 := v.Env().RemoteHost.MustExecute("cat test/test-folder/file-0")
	output1 := v.Env().RemoteHost.MustExecute("cat test/test-folder/folder-1/file-1")

	require.Equal(v.T(), "This is a test file 0", output0)
	require.Equal(v.T(), "This is a test file 1", output1)
}

func createFolders(folder string) error {
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
	return nil
}
