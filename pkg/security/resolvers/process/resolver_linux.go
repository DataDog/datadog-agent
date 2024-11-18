// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const memfdPrefix = "memfd:"

// IsKThread returns whether given pids are from kthreads
func IsKThread(ppid, pid uint32) bool {
	return ppid == 2 || pid == 2
}

// IsBusybox returns true if the pathname matches busybox
func IsBusybox(pathname string) bool {
	return pathname == "/bin/busybox" || pathname == "/usr/bin/busybox"
}

func setPathname(fileEvent *model.FileEvent, pathnameStr string) {
	baseName := path.Base(pathnameStr)
	if fileEvent.FileFields.IsFileless() {
		fileEvent.SetPathnameStr("")
		if !strings.HasPrefix(baseName, memfdPrefix) {
			baseName = memfdPrefix + baseName
		}
	} else {
		fileEvent.SetPathnameStr(pathnameStr)
	}
	fileEvent.SetBasenameStr(baseName)
}

// GetProcessArgv returns the unscrubbed args of the event as an array. Use with caution.
func GetProcessArgv(pr *model.Process) ([]string, bool) {
	if pr.ArgsEntry == nil {
		return pr.Argv, pr.ArgsTruncated
	}

	argv := pr.ArgsEntry.Values
	if len(argv) > 0 {
		argv = argv[1:]
	}
	pr.Argv = argv
	pr.ArgsTruncated = pr.ArgsTruncated || pr.ArgsEntry.Truncated
	return pr.Argv, pr.ArgsTruncated
}

// GetProcessArgv0 returns the first arg of the event and whether the process arguments are truncated
func GetProcessArgv0(pr *model.Process) (string, bool) {
	if pr.ArgsEntry == nil {
		return pr.Argv0, pr.ArgsTruncated
	}

	argv := pr.ArgsEntry.Values
	if len(argv) > 0 {
		pr.Argv0 = argv[0]
	}
	pr.ArgsTruncated = pr.ArgsTruncated || pr.ArgsEntry.Truncated
	return pr.Argv0, pr.ArgsTruncated
}
