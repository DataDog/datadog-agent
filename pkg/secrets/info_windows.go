// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build secrets,windows

package secrets

import (
	"bytes"
	"fmt"
	"os/exec"
)

func (info *SecretInfo) populateRights() {
	for x := 0; x < len(info.ExecutablePath); x++ {
		err := checkRights(info.ExecutablePath[x])
		if err != nil {
			info.Rights = append(info.Rights, fmt.Sprintf("Error: %s", err))
		} else {
			info.Rights = append(info.Rights, fmt.Sprintf("OK, the executable has the correct rights"))
		}

		ps, err := exec.LookPath("powershell.exe")
		if err != nil {
			info.RightDetails = append(info.Rights, fmt.Sprintf("Could not find executable powershell.exe: %s", err))
			return
		}

		cmd := exec.Command(ps, "get-acl", "-Path", info.ExecutablePath, "|", "format-list")

		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		var tmp = ""
		if err != nil {
			tmp += fmt.Sprintf("Error calling 'get-acl': %s\n", err)
		} else {
			tmp += fmt.Sprintf("Acl list:\n")
		}
		tmp += fmt.Sprintf("stdout:\n %s\n", stdout.String())
		if stderr.Len() != 0 {
			tmp += fmt.Sprintf("stderr:\n %s\n", stderr.String())
		}

		info.RightDetails = append(info.RightDetails, tmp)

	}
}
