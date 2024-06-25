// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/require"
)

// RemoteExecutable is a helper struct to run tests on a remote host
type RemoteExecutable struct {
	t         *testing.T
	vm        *components.RemoteHost
	filespec  string   // unqualified name of test executable to be copied.
	basepath  string   // basepath is in host local path separators
	testfiles []string // this is also in host local path separators

}

const remoteDirBase = "c:\\tmp"

// NewRemoteExecutable creates a new RemoteExecutable
func NewRemoteExecutable(vm *components.RemoteHost, t *testing.T, filespec string, basepath string) *RemoteExecutable {
	return &RemoteExecutable{
		vm:       vm,
		t:        t,
		filespec: filespec,
		basepath: basepath,
	}
}

// FindTestPrograms finds the locally available test programs.
//
// it walks the root directory (provided at initialization), looking for any file that matches the provided
// filespec (usually `testsuite.exe`).  If it finds that, it stores the full path for future copying.
func (rs *RemoteExecutable) FindTestPrograms() error {
	// find test programs

	err := filepath.Walk(rs.basepath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			rs.t.Fatalf("Error walking path: %v", err)
		}
		if !info.IsDir() && strings.Contains(info.Name(), rs.filespec) {
			rp, err := filepath.Rel(rs.basepath, path)
			if err != nil {
				return err
			}
			rs.testfiles = append(rs.testfiles, rp)
		}
		return nil
	})

	return err
}

// CreateRemotePaths creates the remote paths for the test programs
//
// Takes the path to the test programs, and creates an identical directory tree on the remote host.
func (rs *RemoteExecutable) CreateRemotePaths() error {
	// create remote paths
	for _, f := range rs.testfiles {
		remotePath := filepath.Join(remoteDirBase, f)
		remoteDir := filepath.Dir(remotePath)

		remoteDir = filepath.ToSlash(remoteDir)
		err := rs.vm.MkdirAll(remoteDir)
		if err != nil {
			rs.t.Logf("Error creating remote directory: %s %v", remoteDir, err)
			return err
		}
		rs.t.Logf("Created remote directory: %s", remoteDir)
	}
	return nil
}

// CopyFiles copies the test programs to the remote host.
//
// CopyFiles also assumes that if there's a "testdata" directory in the same directory as the test program,
// that that should be copied too.
func (rs *RemoteExecutable) CopyFiles() error {
	// copy files
	for _, f := range rs.testfiles {
		remotePath := filepath.Join(remoteDirBase, f)
		remoteDir := filepath.Dir(remotePath)

		remoteFile := filepath.ToSlash(remotePath)
		lfile := filepath.Join(rs.basepath, f)

		rs.t.Logf("Copying %s to %s", lfile, remoteFile)
		rs.vm.CopyFile(lfile, remoteFile)

		// check to see if there's a "testdata" directory attached.  If so, those
		// files need to go along too
		localDir := filepath.Dir(lfile)
		testdata := filepath.Join(localDir, "testdata")
		td, err := os.Stat(testdata)
		if err == nil && td.IsDir() {
			rs.t.Logf("Copying testdata dir %s to %s", testdata, filepath.Join(remoteDir, "testdata"))
			remoteTestData := filepath.ToSlash(filepath.Join(remoteDir, "testdata"))
			if err := rs.vm.CopyFolder(testdata, remoteTestData); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunTests iterates through all of the tests that were copied and executes them one by one.
// it captures the output, and logs it.
func (rs *RemoteExecutable) RunTests() error {

	for _, testsuite := range rs.testfiles {
		rs.t.Logf("Running testsuite: %s", testsuite)
		remotePath := filepath.Join(remoteDirBase, testsuite) //, "testsuite.exe")

		// google test programs compiled in this way run with no timeout by default.
		// don't allow an individual test to take too long
		executeAndLogOutput(rs.t, rs.vm, remotePath, "\"-test.v\"", "\"-test.timeout=2m\"")
	}
	return nil
}

func executeAndLogOutput(t *testing.T, vm *components.RemoteHost, command string, args ...string) {
	cmdDir := filepath.Dir(command)
	outfilename := command + ".out"
	fullcommand := "cd " + cmdDir + ";"
	fullcommand += command + " " + strings.Join(args, " ") + " | Out-File -Encoding ASCII -FilePath " + outfilename
	_, err := vm.Execute(fullcommand)
	require.NoError(t, err)

	// get the output
	outbytes, err := vm.ReadFile(outfilename)
	require.NoError(t, err)

	// log the output
	for _, line := range strings.Split(string(outbytes[:]), "\n") {
		t.Logf("TestSuite: %s", line)
	}
}
