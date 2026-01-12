// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ShortFilePath returns the short path of the file that the log message was emitted from.
func ShortFilePath(frame runtime.Frame) string {
	return ExtractShortPathFromFullPath(frame.File)
}

// ExtractShortPathFromFullPath extracts the short path from a full path.
//
// It is exported to be used from pkg/util/log/setup, and can be unexported once seelog is removed.
func ExtractShortPathFromFullPath(fullPath string) string {
	shortPath := ""
	if strings.Contains(fullPath, "-agent/") {
		// We want to trim the part containing the path of the project
		// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
		slices := strings.Split(fullPath, "-agent/")
		shortPath = slices[len(slices)-1]
	} else {
		// For logging from dependencies, we want to log e.g.
		// "collector@v0.35.0/service/collector.go"
		slices := strings.Split(fullPath, "/")
		atSignIndex := len(slices) - 1
		for ; atSignIndex > 0; atSignIndex-- {
			if strings.Contains(slices[atSignIndex], "@") {
				break
			}
		}
		shortPath = strings.Join(slices[atSignIndex:], "/")
	}
	return shortPath
}

// RelFile removes the working directory from the full path.
//
// See https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/common_context.go#L45-L48
// and shortPath in https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/common_context.go#L100-L106
func RelFile(frame runtime.Frame) string {
	workingDir := "/"
	wd, err := os.Getwd()
	if err == nil {
		workingDir = filepath.ToSlash(wd) + "/"
	}

	return strings.TrimPrefix(frame.File, workingDir)
}
