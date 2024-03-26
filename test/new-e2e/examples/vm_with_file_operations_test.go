// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"io/fs"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type vmSuiteWithFileOperations struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMSuiteWithFileOperations runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuiteWithFileOperations(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))))}
	e2e.Run(t, &vmSuiteWithFileOperations{}, suiteParams...)
}

func (v *vmSuiteWithFileOperations) TestFileOperations() {
	vm := v.Env().RemoteHost
	testFilePath := "test"

	v.T().Cleanup(func() {
		_ = vm.Remove(testFilePath)
	})

	// file does not exist to start
	exists, err := vm.FileExists(testFilePath)
	v.Require().NoError(err)
	v.Require().False(exists)

	// Should return error when file does not exist
	_, err = vm.ReadFile(testFilePath)
	v.Require().ErrorIs(err, fs.ErrNotExist)
	_, err = vm.Lstat(testFilePath)
	v.Require().ErrorIs(err, fs.ErrNotExist)

	// Write to the file
	testContent := []byte("content")
	bytesWritten, err := vm.WriteFile(testFilePath, testContent)
	v.Require().NoError(err)
	v.Require().EqualValues(len(testContent), bytesWritten)

	// Writing to the file should create it
	exists, err = vm.FileExists(testFilePath)
	v.Require().NoError(err)
	v.Require().True(exists)

	fileInfo, err := vm.Lstat(testFilePath)
	v.Require().NoError(err)
	v.Require().False(fileInfo.IsDir())

	// Read content back and ensure it matches
	content, err := vm.ReadFile(testFilePath)
	v.Require().NoError(err)
	v.Require().Equal(testContent, content)

	// Remove the file
	err = vm.Remove(testFilePath)
	v.Require().NoError(err)

	// File should no longer exist
	exists, err = vm.FileExists(testFilePath)
	v.Require().NoError(err)
	v.Require().False(exists)

	// Removing the file again should return an error
	err = vm.Remove(testFilePath)
	v.Require().ErrorIs(err, fs.ErrNotExist)
}

func (v *vmSuiteWithFileOperations) TestDirectoryOperations() {
	vm := v.Env().RemoteHost
	testDirPath := "testDirectory"
	testSubDirPath := "testDirectory/testSubDirectory"

	v.T().Cleanup(func() {
		_ = vm.RemoveAll(testDirPath)
	})

	// directory does not exist to start
	exists, err := vm.FileExists(testDirPath)
	v.Require().NoError(err)
	v.Require().False(exists)

	// Should return error when directory does not exist
	_, err = vm.ReadDir(testDirPath)
	v.Require().ErrorIs(err, fs.ErrNotExist)
	_, err = vm.Lstat(testDirPath)
	v.Require().ErrorIs(err, fs.ErrNotExist)

	// Create the sub directory
	err = vm.MkdirAll(testSubDirPath)
	v.Require().NoError(err)

	// Should create entire directory path
	fileInfo, err := vm.Lstat(testDirPath)
	v.Require().NoError(err)
	v.Require().True(fileInfo.IsDir())
	fileInfo, err = vm.Lstat(testSubDirPath)
	v.Require().NoError(err)
	v.Require().True(fileInfo.IsDir())

	// Removing non-empty directory should return an error
	err = vm.Remove(testDirPath)
	v.Require().Error(err)

	// Removing empty directory should not return an error
	err = vm.Remove(testSubDirPath)
	v.Require().NoError(err)

	// Sub directory should no longer exist
	_, err = vm.Lstat(testSubDirPath)
	v.Require().ErrorIs(err, fs.ErrNotExist)

	// Create the sub directory again
	err = vm.MkdirAll(testSubDirPath)
	v.Require().NoError(err)

	// Should create subdir
	fileInfo, err = vm.Lstat(testSubDirPath)
	v.Require().NoError(err)
	v.Require().True(fileInfo.IsDir())

	// Remove the directory tree
	err = vm.RemoveAll(testDirPath)
	v.Require().NoError(err)

	// Directory should no longer exist
	_, err = vm.Lstat(testDirPath)
	v.Require().ErrorIs(err, fs.ErrNotExist)

	// Removing the directory again should return an error
	err = vm.Remove(testDirPath)
	v.Require().ErrorIs(err, fs.ErrNotExist)
}
