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
	"strings"

	"golang.org/x/sys/windows"
)

// filePermsInfo represents file rights on windows.
type filePermsInfo struct {
	path   string
	mode   string
	icacls string
	err    error
}

func run_cmd(cmd *exec.Cmd) string {
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

func (p permissionsInfos) add(filePath string) {
	info := filePermsInfo{
		path: filePath,
	}
	p[filePath] = &info

	// This is a performance sensitive function because it is invoked
	// directly (via fb.RegisterFilePerm(security.GetAuthTokenFilepath())
	// and indirectly (via fb.permsInfos.add(srcFile)). For example adding
	// complementary output of the following
	//     exec.Command(ps, "get-acl", "-Path", filePathQuoted, "|", "fl")
	// command adds ~60 seconds for added ~150 flare files. Current overhead
	// appears to be in seconds. If it is too slow a call to "icacls.exe"
	// should be replaced using Windows security API invocations.
	fi, err := os.Stat(filePath)
	if err != nil {
		info.err = fmt.Errorf("could not find file %s: %s", filePath, err)
		return
	}
	info.mode = fi.Mode().String()

	// Run icacls.exe
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		info.icacls = fmt.Sprintf("Error: cannot locate icacls.exe %s", err)
	} else {
		execPath := fmt.Sprintf("%s\\icacls.exe", sysDir)
		// Direct path argument will be quoted by the exec.Command https://pkg.go.dev/os/exec#Command
		info.icacls = run_cmd(exec.Command(execPath, filePath))
	}
}

// Commit resolves the infos of every stacked files in the map
// and then writes the permissions.log file on the filesystem.
func (p permissionsInfos) commit() ([]byte, error) {
	f := &bytes.Buffer{}

	sep := strings.Repeat("-", 48) + "\n"

	// write each file permissions infos
	for _, info := range p {
		if _, err := f.Write([]byte(sep)); err != nil {
			return nil, err
		}

		// Print error and try next item
		if info.err != nil {
			_, err := f.Write([]byte(
				fmt.Sprintf("File: %s\n\nError:%s\n\n",
					info.path,
					info.err.Error(),
				)))
			if err != nil {
				return f.Bytes(), err
			}
		} else {
			// ... or actual permissions
			_, err := f.Write([]byte(
				fmt.Sprintf("File: %s\n\nMode:%s\n\n%s\n",
					info.path,
					info.mode,
					info.icacls,
				)))
			if err != nil {
				return f.Bytes(), err
			}
		}
	}

	return f.Bytes(), nil
}
