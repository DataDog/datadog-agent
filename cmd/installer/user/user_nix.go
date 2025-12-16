// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

// Package user provides helpers to change the user of the process.
package user

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
)

// ErrRootRequired is the error returned when an operation requires root privileges.
var ErrRootRequired = errors.New("operation requires root privileges")

// IsRoot returns true if the process is running as root.
func IsRoot() bool {
	return syscall.Getuid() == 0
}

// RootToDatadogAgent changes the user of the process to the Datadog Agent user from root.
// Note that we actually only set dd-agent as the effective user, not the real user, in oder to
// escalate privileges back when needed.
func RootToDatadogAgent() error {
	gid, err := user.GetGroupID(context.Background(), "dd-agent")
	if err != nil {
		return fmt.Errorf("failed to lookup dd-agent group: %s", err)
	}
	err = syscall.Setegid(gid)
	if err != nil {
		return fmt.Errorf("failed to setegid: %s", err)
	}
	uid, err := user.GetUserID(context.Background(), "dd-agent")
	if err != nil {
		return fmt.Errorf("failed to lookup dd-agent user: %s", err)
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
