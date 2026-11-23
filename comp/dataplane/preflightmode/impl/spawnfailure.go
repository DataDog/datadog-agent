// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"errors"
	"io/fs"
	"syscall"
)

// spawnStage is the step at which a run failed before ADP was running. It is shipped as a
// telemetry label on findingSpawnFailed, so the set is closed and code-defined.
//
// spawn_failed covers every step between finding the binary and starting the process. The steps
// fail for unrelated reasons — a read-only run_path, a run_path too long for a unix socket, a
// binary on a noexec mount — and the stage is what tells them apart in fleet telemetry.
type spawnStage string

const (
	spawnStageBinary             spawnStage = "binary"              // the installed binary is unusable
	spawnStageWorkDir            spawnStage = "workdir"             // the working directory could not be created
	spawnStageWorkDirPermissions spawnStage = "workdir_permissions" // the working directory could not be restricted
	spawnStageSocketPath         spawnStage = "socket_path"         // the DogStatsD socket path is too long
	spawnStageHostname           spawnStage = "hostname"            // the Agent's hostname could not be resolved
	spawnStageConfig             spawnStage = "config"              // the generated config could not be written
	spawnStageStart              spawnStage = "start"               // the operating system refused to start the process
	spawnStageUnknown            spawnStage = "unknown"             // an error that no stage claimed
)

// allSpawnStages exists so a test can assert the set has not grown unnoticed.
var allSpawnStages = []spawnStage{
	spawnStageBinary,
	spawnStageWorkDir,
	spawnStageWorkDirPermissions,
	spawnStageSocketPath,
	spawnStageHostname,
	spawnStageConfig,
	spawnStageStart,
	spawnStageUnknown,
}

// spawnErrorClass is a coarse class of the error behind a spawn failure. It is shipped as a
// telemetry label, so it is derived from the error's type and errno only, never from its text,
// which carries paths and other host-specific detail.
type spawnErrorClass string

const (
	spawnErrorPermissionDenied spawnErrorClass = "permission_denied" // EACCES, EPERM, or ERROR_ACCESS_DENIED
	spawnErrorReadOnly         spawnErrorClass = "read_only"         // EROFS
	spawnErrorNotFound         spawnErrorClass = "not_found"         // ENOENT, or ERROR_FILE_NOT_FOUND and ERROR_PATH_NOT_FOUND
	spawnErrorNoSpace          spawnErrorClass = "no_space"          // ENOSPC
	spawnErrorNameTooLong      spawnErrorClass = "name_too_long"     // ENAMETOOLONG
	spawnErrorTextBusy         spawnErrorClass = "text_busy"         // ETXTBSY
	spawnErrorExecFormat       spawnErrorClass = "exec_format"       // ENOEXEC
	spawnErrorNotExecutable    spawnErrorClass = "not_executable"    // the binary has no execute bit
	spawnErrorNotRegularFile   spawnErrorClass = "not_regular_file"  // the binary path is not a regular file
	spawnErrorOther            spawnErrorClass = "other"             // anything else
)

// allSpawnErrorClasses exists so a test can assert the set has not grown unnoticed.
var allSpawnErrorClasses = []spawnErrorClass{
	spawnErrorPermissionDenied,
	spawnErrorReadOnly,
	spawnErrorNotFound,
	spawnErrorNoSpace,
	spawnErrorNameTooLong,
	spawnErrorTextBusy,
	spawnErrorExecFormat,
	spawnErrorNotExecutable,
	spawnErrorNotRegularFile,
	spawnErrorOther,
}

// errNotExecutable and errNotRegularFile are the checks findDataPlane makes itself, rather than
// errors from the operating system, so they get sentinels of their own to classify.
var (
	errNotExecutable  = errors.New("not executable")
	errNotRegularFile = errors.New("not a regular file")
)

// stagedError is an error tagged with the spawn stage it happened at.
type stagedError struct {
	stage spawnStage
	err   error
}

func (e *stagedError) Error() string { return e.err.Error() }
func (e *stagedError) Unwrap() error { return e.err }

// atStage tags err with the stage it happened at.
func atStage(stage spawnStage, err error) error {
	return &stagedError{stage: stage, err: err}
}

// spawnFailure is what the finding metric reports for findingSpawnFailed.
type spawnFailure struct {
	stage spawnStage
	class spawnErrorClass
}

// newSpawnFailure classifies err. An error without a stage reports spawnStageUnknown, so a new
// failure path that forgets atStage still shows up in telemetry, and in a form that points at
// the omission.
func newSpawnFailure(err error) spawnFailure {
	stage := spawnStageUnknown
	var staged *stagedError
	if errors.As(err, &staged) {
		stage = staged.stage
	}
	return spawnFailure{stage: stage, class: classifySpawnError(err)}
}

// classifySpawnError maps err onto a spawnErrorClass.
//
// The fs sentinels come first because they are what makes the check portable: on Windows,
// errors.Is maps ERROR_ACCESS_DENIED onto fs.ErrPermission and the not-found codes onto
// fs.ErrNotExist. The errno comparisons after them only ever match on Unix. Windows reports
// those conditions through codes of its own, and those fall through to spawnErrorOther.
func classifySpawnError(err error) spawnErrorClass {
	switch {
	case errors.Is(err, errNotExecutable):
		return spawnErrorNotExecutable
	case errors.Is(err, errNotRegularFile):
		return spawnErrorNotRegularFile
	case errors.Is(err, syscall.EROFS):
		return spawnErrorReadOnly
	case errors.Is(err, fs.ErrPermission):
		return spawnErrorPermissionDenied
	case errors.Is(err, fs.ErrNotExist):
		return spawnErrorNotFound
	case errors.Is(err, syscall.ENOSPC):
		return spawnErrorNoSpace
	case errors.Is(err, syscall.ENAMETOOLONG):
		return spawnErrorNameTooLong
	case errors.Is(err, syscall.ETXTBSY):
		return spawnErrorTextBusy
	case errors.Is(err, syscall.ENOEXEC):
		return spawnErrorExecFormat
	default:
		return spawnErrorOther
	}
}
