// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package user provides helpers to change the user of the process.
package user

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// ErrRootRequired is the error returned when an operation requires Administrator privileges.
var ErrRootRequired = errors.New("operation requires Administrator privileges")

// IsRoot returns true if token has Administrators group enabled
func IsRoot() bool {
	isAdmin, err := winutil.IsUserAnAdmin()
	if err != nil {
		fmt.Printf("error checking if user is admin: %v\n", err)
	}
	return isAdmin
}

// RootToDatadogAgent is a noop on windows.
func RootToDatadogAgent() error {
	return nil
}

// DatadogAgentToRoot is a noop on windows.
func DatadogAgentToRoot() error {
	return nil
}
