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
//
// When go_crash_report is enabled, Setup also logs the Go crash output left by
// the previous run (if any) and saves the Go crash output of the current run to
// a file in go_crash_report_dir. Crash report errors are logged as warnings and
// are not returned. Call Setup after the logger is initialized, so that the
// report is logged.
func Setup(cfg model.Reader) error {
	if cfg.GetBool("go_crash_report") {
		setupCrashReport(cfg.GetString("go_crash_report_dir"), processName())
	}

	if cfg.GetBool("go_core_dump") {
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
