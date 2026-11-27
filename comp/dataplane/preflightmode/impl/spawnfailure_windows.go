// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package preflightmodeimpl

import (
	"errors"
	"syscall"
)

// Windows system error codes that have a spawnErrorClass but no fs sentinel. Defined here
// rather than imported, because the syscall package does not export them.
const (
	errorWriteProtect           = syscall.Errno(19)   // ERROR_WRITE_PROTECT
	errorHandleDiskFull         = syscall.Errno(39)   // ERROR_HANDLE_DISK_FULL
	errorDiskFull               = syscall.Errno(112)  // ERROR_DISK_FULL
	errorBadExeFormat           = syscall.Errno(193)  // ERROR_BAD_EXE_FORMAT
	errorFilenameExcedRange     = syscall.Errno(206)  // ERROR_FILENAME_EXCED_RANGE
	errorExeMachineTypeMismatch = syscall.Errno(216)  // ERROR_EXE_MACHINE_TYPE_MISMATCH
	errorDiskQuotaExceeded      = syscall.Errno(1295) // ERROR_DISK_QUOTA_EXCEEDED
)

// classifyPlatformSpawnError maps the Windows error codes for conditions that Unix reports
// through an errno classifySpawnError already checks.
func classifyPlatformSpawnError(err error) (spawnErrorClass, bool) {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return "", false
	}
	switch errno {
	case errorWriteProtect:
		return spawnErrorReadOnly, true
	case errorHandleDiskFull, errorDiskFull, errorDiskQuotaExceeded:
		return spawnErrorNoSpace, true
	case errorBadExeFormat, errorExeMachineTypeMismatch:
		return spawnErrorExecFormat, true
	case errorFilenameExcedRange:
		return spawnErrorNameTooLong, true
	default:
		return "", false
	}
}
