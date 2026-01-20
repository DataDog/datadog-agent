// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package filesystem

import (
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func SetCorrectRight(path string) {
	// error not checked
	_ = exec.Command("powershell", "test/setAcl.ps1",
		"-file", path,
		"-removeAllUser", "1",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
}

func TestCheckRightsStub() {
	// Stub for CI since running as Administrator and no installer data
	getDDAgentUserSID = winutil.GetSidFromUser
}
