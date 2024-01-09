// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"flag"
	"io/fs"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

var (
	devMode = flag.Bool("devmode", false, "enable dev mode")
)

type vmSuite struct {
	e2e.Suite[e2e.VMEnv]
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuite(t *testing.T) {
	suiteParams := make([]func(*params.Params), 0)
	if *devMode {
		suiteParams = append(suiteParams, params.WithDevMode())
	}
	e2e.Run(t, &vmSuite{},
		e2e.EC2VMStackDef(ec2params.WithOS(ec2os.WindowsOS)),
		suiteParams...,
	)
}

func (v *vmSuite) TestExecute() {
	vm := v.Env().VM

	out, err := vm.ExecuteWithError("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}

func (v *vmSuite) TestFileOperations() {
	vm := v.Env().VM
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

func (v *vmSuite) TestDirectoryOperations() {
	vm := v.Env().VM
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
