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

type RemoteSuite struct {
	t         *testing.T
	vm        *components.RemoteHost
	filespec  string   // unqualified name of test executable to be copied.
	basepath  string   // basepath is in host local path separators
	testfiles []string // this is also in host local path separators

}

const remoteDirBase = "c:\\tmp"

func NewRemoteSuite(vm *components.RemoteHost, t *testing.T, filespec string, basepath string) *RemoteSuite {
	return &RemoteSuite{
		vm:       vm,
		t:        t,
		filespec: filespec,
		basepath: basepath,
	}
}
func (rs *RemoteSuite) FindTestPrograms() error {
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

func (rs *RemoteSuite) CreateRemotePaths() error {
	// create remote paths
	for _, f := range rs.testfiles {
		remotePath := filepath.Join(remoteDirBase, f)
		remoteDir := filepath.Dir(remotePath)

		remoteDir = filepath.ToSlash(remoteDir)
		err := rs.vm.MkdirAll(remoteDir)
		if err != nil {
			rs.t.Logf("Error creating remote directory: %s %v", remoteDir, err)
			return err
		} else {
			rs.t.Logf("Created remote directory: %s", remoteDir)
		}
	}
	return nil
}

func (rs *RemoteSuite) CopyFiles() error {
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
			rs.vm.CopyFolder(testdata, remoteTestData)
		}
	}
	return nil
}

func (rs *RemoteSuite) RunTests() error {

	for _, testsuite := range rs.testfiles {
		rs.t.Logf("Running testsuite: %s", testsuite)
		remotePath := filepath.Join(remoteDirBase, testsuite) //, "testsuite.exe")
		executeAndLogOutput(rs.t, rs.vm, remotePath, "\"-test.v\"")
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
