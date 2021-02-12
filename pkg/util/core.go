// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package util

import (
	"fmt"
	"runtime/debug"

	"github.com/DataDog/datadog-agent/pkg/config"
	"golang.org/x/sys/unix"
)

// SetupCoreDump enables core dumps and sets the core dump size limit based on configuration
func SetupCoreDump() error {
	if config.Datadog.GetBool("go_core_dump") {
		debug.SetTraceback("crash")

		err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{
			Cur: unix.RLIM_INFINITY,
			Max: unix.RLIM_INFINITY,
		})

		if err != nil {
			return fmt.Errorf("Failed to set ulimit for core dumps: %s", err)
		}
	}

	return nil
}
