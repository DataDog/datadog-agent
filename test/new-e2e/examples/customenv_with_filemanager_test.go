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
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type fileManagerSuiteEx7 struct {
	e2e.Suite[e2e.VMEnv]
}

func fileManagerVMStackDef(localFolderPath string, remoteFolderPath string) *e2e.StackDefinition[e2e.VMEnv] {
	return e2e.EnvFactoryStackDef(func(ctx *pulumi.Context) (*e2e.VMEnv, error) {
		vm, err := ec2vm.NewEc2VM(ctx, ec2params.WithOS(ec2os.UbuntuOS))
		if err != nil {
			return nil, err
		}

		fileManager := vm.GetFileManager()
		fileManager.CopyAbsoluteFolder(localFolderPath, remoteFolderPath)

		return &e2e.VMEnv{
			VM: client.NewVM(vm),
		}, nil
	})
}

func TestFileManagerSuiteEx7(t *testing.T) {
	testFolder := path.Join(os.TempDir(), "test-folder")
	createFolders(testFolder)
	e2e.Run(t, &fileManagerSuiteEx7{}, fileManagerVMStackDef(testFolder, "/home/ubuntu/test"))
}

func (v *fileManagerSuiteEx7) TestCopy() {

	output0 := v.Env().VM.Execute("cat test/test-folder/file-0")
	output1 := v.Env().VM.Execute("cat test/test-folder/folder-1/file-1")

	require.Equal(v.T(), "This is a test file 0", output0)
	require.Equal(v.T(), "This is a test file 1", output1)

}

func createFolders(folder string) error {

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
	return nil
}
