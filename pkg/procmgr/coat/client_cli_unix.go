// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package coat

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

func procmgrCLIPath(_ string) string {
	return filepath.Join(defaultpaths.GetEmbeddedBinPath(), "dd-procmgr")
}

func runAsDDAgent(cmd *exec.Cmd) error {
	if os.Geteuid() != 0 {
		return nil
	}
	ddAgent, err := user.Lookup("dd-agent")
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(ddAgent.Uid, 10, 32)
	if err != nil {
		return err
	}
	gid, err := strconv.ParseUint(ddAgent.Gid, 10, 32)
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}}
	return nil
}
