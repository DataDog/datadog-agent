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
}

func terminateProcess(cmd *exec.Cmd) {
	err := cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		log.Errorf("Failed to send SIGTERM to %s (%d): %v", secretBackendCommand, cmd.Process.Pid, err)
	}
}
