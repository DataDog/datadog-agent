// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pidfile

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// isProcess checks to see if a given pid is currently valid in the process table
func isProcess(pid int) bool {
	return winutil.IsProcess(pid)
}

// Path returns a suitable location for the pidfile under Windows
func Path() string {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		return filepath.Join(pd, "datadog-agent.pid")
	}
	return "c:\\ProgramData\\DataDog\\datadog-agent.pid"
}
