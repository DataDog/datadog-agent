// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package user provides helpers to change the user of the process.
package user

import (
	"errors"
	"syscall"
)

// ErrRootRequired is the error returned when an operation requires root privileges.
var ErrRootRequired = errors.New("operation requires root privileges")

// IsRoot always returns true on darwin.
func IsRoot() bool {
	return syscall.Getuid() == 0
}

// RootToDatadogAgent is a noop on darwin.
func RootToDatadogAgent() error {
	return nil
}

// DatadogAgentToRoot is a noop on darwin.
func DatadogAgentToRoot() error {
	return nil
}
