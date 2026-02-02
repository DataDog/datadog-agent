// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build windows && test

package filesystem

import (
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func boolToString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// SetACL modifies the permissions of the specified file
func SetACL(path string, removeAllUser bool, removeAdmin bool, removeLocalSystem bool, addDDUser bool) error {
	return exec.Command("powershell", "test/setAcl.ps1",
		"-file", path,
		"-removeAllUser", boolToString(removeAllUser),
		"-removeAdmin", boolToString(removeAdmin),
		"-removeLocalSystem", boolToString(removeLocalSystem),
		"-addDDuser", boolToString(addDDUser),
	).Run()
}

func SetCorrectRight(path string) error {
	return SetACL(path, true, false, false, true)
}

func TestCheckRightsStub() {
	// Stub for CI since running as Administrator and no installer data
	getDDAgentUserSID = winutil.GetSidFromUser
}
