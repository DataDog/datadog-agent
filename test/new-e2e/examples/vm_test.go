// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"flag"
	"io/fs"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

var devMode = flag.Bool("devmode", false, "enable dev mode")

type vmSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))))}
	if *devMode {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}

func (v *vmSuite) TestFileOperations() {
	vm := v.Env().RemoteHost
	// Use drive letter path with forward slashes to ensure Windows paths are handled correctly
	testFilePath := "C:\\testFile"

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

func (v *vmSuite) TestDirectoryOperations() {
	vm := v.Env().RemoteHost
	// Use drive letter path with forward slashes to ensure Windows paths are handled correctly
	testDirPath := "C:\\testDirectory"
	testSubDirPath := "C:\\testDirectory\\testSubDirectory"

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
