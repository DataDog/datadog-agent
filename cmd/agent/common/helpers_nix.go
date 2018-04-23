// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build freebsd netbsd openbsd solaris dragonfly linux darwin

package common

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	procfsPathEnv = "HOST_PROC"
)

// SetupConfigOSSpecifics any additional OS-specific configuration necessary
// should be called _after_ SetupConfig()
func SetupConfigOSSpecifics() error {
	procfsPath := os.Getenv(procfsPathEnv)
	if procfsPath != "" && config.Datadog.IsSet("procfs_path") {
		// override with HOST_PROC if set
		config.Datadog.Set("procfs_path", procfsPath)
	}

	return nil
}
