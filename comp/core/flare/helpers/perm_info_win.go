// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
)

// filePermsInfo represents file rights on windows.
type filePermsInfo struct{}

func runCmd(cmd *exec.Cmd) string {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Sprintf("Error calling '%s': %s", cmd.String(), err)
	}

	output := fmt.Sprintf("Cmd: %s\n%s", cmd.String(), stdout.String())
	if stderr.Len() != 0 {
		output += fmt.Sprintf("\n%s", stderr.String())
	}

	return output
}

// Get Datadog bin directory
func getDatadogProgramFilesBinPath() (string, error) {
	// By default C:\Program Files\Datadog\Datadog Agent
	installDir, err := winutil.GetProgramFilesDirForProduct("DataDog Agent")
	if err != nil {
		return "", err
	}

	// By default C:\Program Files\Datadog\Datadog Agent\bin
	return filepath.Join(installDir, "bin"), nil
}

// Virtually all files whose permissions we are trying to collect come from
// the Datadog configuration directory (at least the one we care about). It
// may change in the future.
func getDatadogProgramDatatPath() string {
	// By default C:\ProgramData\Datadog
	dir := filepath.Dir(config.Datadog.ConfigFileUsed())

	// Handle unit test run
	if dir == "." {
		dir, _ = os.Getwd()
	}

	return dir
}

func getPermExeFilePath() string {
	sysDir, _ := windows.GetSystemDirectory()
	return fmt.Sprintf("%s\\icacls.exe", sysDir)
}

func (p permissionsInfos) add(filePath string) {
	// Instead of tracking permissions for an individual file we currently capture
	// permissions for all the files in the configuration directory in the commit
	// method below. Because icacls.exe is used for permission collection, it runs
	// much faster for even large directories than for dozens of individual files.
	// In the future it is still more efficient to use Windows API to collect
	// permissions in binary format and translate it to human readable form but
	// for now it is relatively performance with 0.4 seconds on collecting
	// permissions for 400 files and saving 150 Kb of output compressed to 7 kb of
	// the zip file. In contrast, permission collection via icacls.exe for 142
	// files individually took 5-12 seconds and generated 90 kb of output
	// compressed to 5 kb.
}

// Commit resolves the infos of every stacked files in the map
// and then writes the permissions.log file on the filesystem.
func (p permissionsInfos) commit() ([]byte, error) {
	f := &bytes.Buffer{}

	var err error

	execPath := getPermExeFilePath()

	// Get Datadog configuration directory
	dir := getDatadogProgramDatatPath()
	cmdOut := runCmd(exec.Command(execPath, dir, "/T"))
	_, err = f.Write([]byte(cmdOut))
	if err != nil {
		return f.Bytes(), err
	}

	// Get Datadog bin directory (optional if err)
	dir, err = getDatadogProgramFilesBinPath()
	if err == nil {
		// Collect privileges for Agent executables
		agentExeFilePathPattern := filepath.Join(dir, "*.exe")
		cmdOut := runCmd(exec.Command(execPath, agentExeFilePathPattern, "/T"))
		_, err = f.Write([]byte(cmdOut))
		if err != nil {
			return f.Bytes(), err
		}
	}

	return f.Bytes(), nil
}
