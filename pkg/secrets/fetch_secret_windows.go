// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

// +build secrets

package secrets

import (
	"os/exec"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func terminateProcess(cmd *exec.Cmd) {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		log.Errorf("LoadDLL: %v\n", err)
		return
	}
	proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		log.Errorf("FindProc: %v\n", err)
		return
	}
	res, _, err := proc.Call(syscall.CTRL_CLOSE_EVENT, uintptr(cmd.Process.Pid))
	if res == 0 {
		log.Errorf("GenerateConsoleCtrlEvent: %v\n", err)
	}
}
