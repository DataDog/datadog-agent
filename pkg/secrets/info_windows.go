// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build secrets,windows

package secrets

import (
	"bytes"
	"fmt"
	"os/exec"
)

func (info *SecretInfo) populateRights() {
	err := checkRights(info.ExecutablePath, secretBackendCommandAllowGroupExec)
	if err != nil {
		info.Rights = fmt.Sprintf("Error: %s", err)
	} else {
		info.Rights = fmt.Sprintf("OK, the executable has the correct rights")
	}

	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		info.RightDetails = fmt.Sprintf("Could not find executable powershell.exe: %s", err)
		return
	}

	cmd := exec.Command(ps, "get-acl", "-Path", info.ExecutablePath, "|", "format-list")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		info.RightDetails += fmt.Sprintf("Error calling 'get-acl': %s\n", err)
	} else {
		info.RightDetails += fmt.Sprintf("Acl list:\n")
	}
	info.RightDetails += fmt.Sprintf("stdout:\n %s\n", stdout.String())
	if stderr.Len() != 0 {
		info.RightDetails += fmt.Sprintf("stderr:\n %s\n", stderr.String())
	}
}
