// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package coat

import (
	"os/exec"
	"path/filepath"
)

func procmgrCLIPath(installRoot string) string {
	return filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
}

func runAsDDAgent(_ *exec.Cmd) error {
	return nil
}
