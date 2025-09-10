// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package secretsimpl

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func (r *secretResolver) getExecutablePermissions() (*permissionsDetails, error) {
	execPath := fmt.Sprintf("\"%s\"", strings.TrimSpace(r.backendCommand))
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return nil, fmt.Errorf("Could not find executable powershell.exe: %s", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	cmd := exec.Command(ps, "get-acl", "-Path", execPath, "|", "format-list")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	details := &permissionsDetails{}
	err = cmd.Run()
	if err != nil {
		details.Error = fmt.Sprintf("Error calling 'get-acl': %s", err)
	}

	if out := strings.TrimSpace(stdout.String()); out != "" {
		details.StdOut = out
	}

	if errOut := strings.TrimSpace(stderr.String()); errOut != "" {
		details.StdErr = errOut
	}
	details.IsWindows = true

	return details, nil
}
