// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && windows

package secrets

import (
	"bytes"
	_ "embed"
	"fmt"
	"os/exec"
	"strings"
)

//go:embed info_win.tmpl
var permissionsDetailsTemplate string

type permissionsDetails struct {
	Error  string
	Stdout string
	Stderr string
}

func getExecutablePermissions() (interface{}, error) {
	execPath := fmt.Sprintf("\"%s\"", strings.TrimSpace(secretBackendCommand))
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return nil, fmt.Errorf("Could not find executable powershell.exe: %s", err)
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	details := permissionsDetails{}

	cmd := exec.Command(ps, "get-acl", "-Path", execPath, "|", "format-list")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		details.Error = fmt.Sprintf("Error calling 'get-acl': %s", err)
	}

	details.Stdout = strings.TrimSpace(stdout.String())

	if stderr.Len() != 0 {
		details.Stderr = strings.TrimSpace(stderr.String())
	}

	return details, nil
}
