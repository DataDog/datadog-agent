// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"time"

	"github.com/Microsoft/go-winio"
)

// NewDaemonChecker creates a new DaemonChecker instance
func NewDaemonChecker() Checker {
	return &daemonCheckerImpl{}
}

func (c *daemonCheckerImpl) IsRunning() (bool, error) {
	timeout := 100 * time.Millisecond
	conn, err := winio.DialPipe("\\\\.\\pipe\\DD_INSTALLER", &timeout)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}
