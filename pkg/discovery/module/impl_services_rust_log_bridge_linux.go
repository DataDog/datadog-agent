// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import "github.com/DataDog/datadog-agent/pkg/util/log"

// rustLevelToGoLevel maps the Rust dd_log_fn level encoding to a Go LogLevel.
// Level encoding: 1=Error, 2=Warn, 3=Info, 4=Debug, 5+=Trace.
func rustLevelToGoLevel(level uint32) log.LogLevel {
	switch level {
	case 1:
		return log.ErrorLvl
	case 2:
		return log.WarnLvl
	case 3:
		return log.InfoLvl
	case 4:
		return log.DebugLvl
	default:
		return log.TraceLvl
	}
}

// handleDiscoveryLog routes a Rust log record to the Go logger.
// Separated from the cgo boundary so it can be unit-tested without the Rust library.
func handleDiscoveryLog(level uint32, message string) {
	goLevel := rustLevelToGoLevel(level)
	if !log.ShouldLog(goLevel) {
		return
	}
	switch level {
	case 1:
		_ = log.Errorf("[dd_discovery] %s", message)
	case 2:
		_ = log.Warnf("[dd_discovery] %s", message)
	case 3:
		log.Infof("[dd_discovery] %s", message)
	case 4:
		log.Debugf("[dd_discovery] %s", message)
	default:
		log.Tracef("[dd_discovery] %s", message)
	}
}

// goLevelToRust maps the current Go log level to the dd_log_fn encoding used by
// dd_discovery_init_logger: 1=Error, 2=Warn, 3=Info, 4=Debug, 5=Trace.
// Defaults to Info if the logger is not yet initialised.
func goLevelToRust() uint32 {
	lvl, err := log.GetLogLevel()
	if err != nil {
		return 3 // Info
	}
	switch lvl {
	case log.ErrorLvl, log.CriticalLvl:
		return 1
	case log.WarnLvl:
		return 2
	case log.InfoLvl:
		return 3
	case log.DebugLvl:
		return 4
	default: // TraceLvl
		return 5
	}
}
