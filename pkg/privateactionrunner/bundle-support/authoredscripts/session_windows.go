// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package authoredscripts

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

func newSessionRoot() (string, error) {
	// Windows does not enforce Unix mode bits, so sessions inherit the Agent run directory's restricted ACLs.
	return os.MkdirTemp(defaultpaths.GetDefaultRunPath(), sessionDirectoryPrefix)
}
