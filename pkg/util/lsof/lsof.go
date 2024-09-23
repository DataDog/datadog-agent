// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lsof provides a way to list open files for a given process
package lsof

import (
	"os"
)

// ListOpenFiles returns a list of open files for the given process
func ListOpenFiles(pid int) (Files, error) {
	return openFiles(pid)
}

// ListOpenFilesFromSelf returns a list of open files for the current process
func ListOpenFilesFromSelf() (Files, error) {
	return ListOpenFiles(os.Getpid())
}
