// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

//nolint:revive // TODO(AGENTRUN) Fix revive linter
package disk

import (
	"bytes"
	"os/exec"
)

var BlkidCommand = func() (string, error) {
	cmd := exec.Command("blkid", []string{}...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

var NetAddConnection = func(mountType, localName, remoteName, _password, _username string) error {
	args := []string{}
	args = append(args, "-t")
	if mountType == "SMB" {
		args = append(args, "smbfs")
	} else {
		args = append(args, mountType)
	}
	args = append(args, remoteName)
	args = append(args, localName)
	cmd := exec.Command("mount", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return err
}
