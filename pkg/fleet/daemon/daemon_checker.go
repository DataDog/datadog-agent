// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import "context"

type daemonCheckerImpl struct{}

// Checker defines the interface for checking the daemon's running state
type Checker interface {
	IsRunning(context.Context) (bool, error)
}

// NewDaemonChecker creates a new DaemonChecker instance
func NewDaemonChecker() Checker {
	return &daemonCheckerImpl{}
}
