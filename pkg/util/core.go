// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package util

import (
	"runtime/debug"

	"github.com/DataDog/datadog-agent/pkg/config"
	"golang.org/x/sys/unix"
)

// SetCoreLimit sets the core dump size limit based on configuration
func SetCoreLimit() error {
	coreSize := uint64(config.Datadog.GetInt("go_core_size"))

	if coreSize > 0 {
		// enable core dump
		debug.SetTraceback("crash")
	}

	err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{
		Cur: coreSize,
		Max: coreSize,
	})

	return err
}
