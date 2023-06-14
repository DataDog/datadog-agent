// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly || linux

package gui

import (
	"fmt"
)

func restartEnabled() bool {
	return false
}

func restart() error {
	return fmt.Errorf("restarting the agent is not implemented on non-windows platforms")
}
