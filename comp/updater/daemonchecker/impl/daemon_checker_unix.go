// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemoncheckerimpl

import (
	"net"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	socketName = "installer.sock"
)

func (c *checkerImpl) IsRunning() (bool, error) {
	conn, err := net.DialTimeout("unix", filepath.Join(paths.RunPath, socketName), 100*time.Millisecond)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}
