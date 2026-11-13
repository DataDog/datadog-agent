// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build !windows

package coredump

import (
	"fmt"
	"runtime/debug"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Setup enables core dumps and sets the core dump size limit based on configuration.
func Setup(cfg model.Reader) error {
	goCoreDump := cfg.GetBool("go_core_dump")
	if goCoreDump {
		debug.SetTraceback("crash")
	}

	if goCoreDump || cfg.GetBool("c_core_dump") {
		err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{
			Cur: unix.RLIM_INFINITY,
			Max: unix.RLIM_INFINITY,
		})

		if err != nil {
			return fmt.Errorf("failed to set ulimit for core dumps: %s", err)
		}
	}

	return nil
}
