// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcmgrCreateSpecCLIArgs(t *testing.T) {
	args := procmgrCreateSpec{
		Name:    "e2e-userprofile-env",
		Command: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		Args: []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"$env:USERPROFILE | Set-Content -LiteralPath 'C:/ProgramData/Datadog/marker.txt'",
		},
		Env: map[string]string{
			"PATH":       `C:\Windows\System32;C:\Windows`,
			"SystemRoot": `C:\Windows`,
		},
		RestartPolicy: "always",
		Description:   "E2E userprofile env check",
	}.cliArgs()

	assert.Contains(t, args, "--args=-NoProfile")
	assert.Contains(t, args, "--args=-NonInteractive")
	assert.Contains(t, args, "--args=-Command")
	assert.Contains(t, args, `--command='C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'`)
	assert.Contains(t, args, `--env='PATH=C:\Windows\System32;C:\Windows'`)
	assert.NotContains(t, args, "--args '-NoProfile'")
	assert.True(t, strings.HasPrefix(args, "create "))
}

func TestProcmgrCreateSpecNoAutoStart(t *testing.T) {
	args := procmgrCreateSpec{
		Name:        "proc",
		Command:     `C:\Windows\System32\cmd.exe`,
		Args:        []string{"/c", "exit"},
		Description: "E2E admin pipe auth",
		NoAutoStart: true,
	}.cliArgs()

	assert.Contains(t, args, "--args=/c")
	assert.Contains(t, args, "--args=exit")
	assert.Contains(t, args, "--no-auto-start")
}
