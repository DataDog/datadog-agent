// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"golang.org/x/crypto/ssh"
	"io/fs"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
)

type vmSuiteWithFileOperations struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMSuiteWithFileOperations runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuiteWithFileOperations(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))))}
	e2e.Run(t, &vmSuiteWithFileOperations{}, suiteParams...)
}

func assertExitCodeEqual(t *testing.T, err error, expected int, msgAndArgs ...interface{}) {
	t.Helper()
	var exitErr *ssh.ExitError
	assert.ErrorAs(t, err, &exitErr)
	assert.Equal(t, expected, exitErr.ExitStatus(), msgAndArgs)
}

// TestCommandResults tests that commands return output or errors in the expected way
func (v *vmSuiteWithFileOperations) TestCommandResults() {
	vm := v.Env().RemoteHost

	// successful command should return the output
	out, err := vm.Execute("echo hello")
	v.Assert().NoError(err)
	v.Assert().Contains(out, "hello")

	// invalid commands should return an error
	_, err = vm.Execute("not-a-command")
	v.Assert().Error(err, "invalid command should return an error")

	if vm.OSFamily == os.WindowsFamily {
		v.testWindowsCommandResults()
	}
}

func (v *vmSuiteWithFileOperations) testWindowsCommandResults() {
	vm := v.Env().RemoteHost

	// invalid commands should return an error
	_, err := vm.Execute("not-a-command")
	v.Assert().Error(err, "invalid command should return an error")
	assertExitCodeEqual(v.T(), err, 1, "generic poewrshell error should return exit code 1")

	// native commands should return the exit status
	_, err = vm.Execute("cmd.exe /c exit 2")
	v.Assert().Error(err, "native command failure should return an error")
	assertExitCodeEqual(v.T(), err, 2, "specific exit code should be returned")

	// a failing native command should continue to execute the rest of the command
	// and the result should be from the lsat command
	out, err := vm.Execute("cmd.exe /c exit 2; echo hello")
	v.Assert().NoError(err, "result should come from the last command")
	v.Assert().Contains(out, "hello", "native command failure should continue to execute the rest of the command")

	// Execute should auto-set $ErrorActionPreference to 'Stop', so
	// a failing PowerShell cmdlet should fail immediately and not
	// execute the rest of the command, so the output should not contain "hello"
	out, err = vm.Execute(`Write-Error 'error'; echo he""llo`)
	v.Assert().Error(err, "Execute should add ErrorActionPreference='Stop' to stop command execution on error")
	v.Assert().NotContains(err.Error(), "hello")
	v.Assert().NotContains(out, "hello")
	assertExitCodeEqual(v.T(), err, 1, "failing PowerShell cmdlet should return exit code 1")

	// Execute should auto-set $ErrorActionPreference to 'Stop', so subcommands return an error
	_, err = vm.Execute(`(Get-Service -Name 'not-a-service').Status`)
	v.Assert().Error(err, "Execute should add ErrorActionPreference='Stop' to stop subcommand execution on error")
	assertExitCodeEqual(v.T(), err, 1, "failing PowerShell cmdlet should return exit code 1")
	// Sanity check default 'Continue' behavior does not return an error
	_, err = vm.Execute(`$ErrorActionPreference='Continue'; (Get-Service -Name 'not-a-service').Status`)
	v.Assert().NoError(err, "explicit ErrorActionPreference='Continue' should ignore subcommand error")
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
