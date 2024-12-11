// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package detectors

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const runtimeDll = "/System.Runtime.dll"

var errorDllNotFound = dderrors.NewNotFound(runtimeDll)

// DotnetDetector detects .NET processes.
type DotnetDetector struct {
	hostProc string
}

// NewDotnetDetector creates a new instance of DotnetDetector.
func NewDotnetDetector() DotnetDetector {
	return DotnetDetector{hostProc: kernel.ProcFSRoot()}
}

// mapsHasDotnetDll checks if the maps file includes a path with the .NET
// runtime DLL.
func mapsHasDotnetDll(reader io.Reader) (bool, error) {
	scanner := bufio.NewScanner(bufio.NewReader(reader))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(line, runtimeDll) {
			return true, nil
		}
	}

	return false, scanner.Err()
}

func (d DotnetDetector) getMapsPath(pid int32) string {
	return path.Join(d.hostProc, strconv.FormatInt(int64(pid), 10), "maps")
}

// DetectLanguage detects if a process is a .NET process.  It does this by using
// /proc/PID/maps to check if the process has mapped a standard .NET dll. This
// works for non-single-file deployments (both self-contained and
// framework-dependent), and framework-dependent single-file deployments.
//
// It does not work for self-contained single-file deployments since these do
// not have any DLLs in their maps file.
func (d DotnetDetector) DetectLanguage(process languagemodels.Process) (languagemodels.Language, error) {
	path := d.getMapsPath(process.GetPid())
	mapsFile, err := os.Open(path)
	if err != nil {
		return languagemodels.Language{}, fmt.Errorf("open: %v", err)
	}
	defer mapsFile.Close()

	hasDLL, err := mapsHasDotnetDll(mapsFile)
	if err != nil {
		return languagemodels.Language{}, err
	}
	if !hasDLL {
		return languagemodels.Language{}, errorDllNotFound
	}

	return languagemodels.Language{
		Name: languagemodels.Dotnet,
	}, nil
}
