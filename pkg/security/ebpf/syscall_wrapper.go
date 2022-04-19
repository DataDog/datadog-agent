// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package ebpf

import (
	"strings"

	manager "github.com/DataDog/ebpf-manager"
)

// IsSyscallWrapperRequired checks if system calls on the current architecture are have the "sys_" prefix
func IsSyscallWrapperRequired() (bool, error) {
	openSyscall, err := manager.GetSyscallFnName("open")
	if err != nil {
		return false, err
	}

	return !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_"), nil
}
