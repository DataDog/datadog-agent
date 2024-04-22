// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package user provides helpers to change the user of the process.
package user

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

// IsRoot returns true if the process is running as root.
func IsRoot() bool {
	return syscall.Getuid() == 0
}

// RootToDatadogAgent changes the user of the process to the Datadog Agent user from root.
// Note that we actually only set dd-agent as the effective user, not the real user, in oder to
// escalate privileges back when needed.
func RootToDatadogAgent() error {
	datadogAgentGroup, err := user.LookupGroup("dd-agent")
	if err != nil {
		return fmt.Errorf("failed to lookup dd-agent group: %s", err)
	}
	gid, err := strconv.Atoi(datadogAgentGroup.Gid)
	if err != nil {
		return fmt.Errorf("failed to convert dd-agent group ID to int: %s", err)
	}
	err = syscall.Setegid(gid)
	if err != nil {
		return fmt.Errorf("failed to setegid: %s", err)
	}
	datadogAgentUser, err := user.Lookup("dd-agent")
	if err != nil {
		return fmt.Errorf("failed to lookup dd-agent user: %s", err)
	}
	uid, err := strconv.Atoi(datadogAgentUser.Uid)
	if err != nil {
		return fmt.Errorf("failed to convert dd-agent user ID to int: %s", err)
	}
	err = syscall.Seteuid(uid)
	if err != nil {
		return fmt.Errorf("failed to seteuid: %s", err)
	}
	return nil
}

// DatadogAgentToRoot changes the user of the process to root from the Datadog Agent user.
func DatadogAgentToRoot() error {
	err := syscall.Setuid(0)
	if err != nil {
		return fmt.Errorf("failed to setuid: %s", err)
	}
	err = syscall.Seteuid(0)
	if err != nil {
		return fmt.Errorf("failed to seteuid: %s", err)
	}
	err = syscall.Setegid(0)
	if err != nil {
		return fmt.Errorf("failed to setgid: %s", err)
	}
	return nil
}
