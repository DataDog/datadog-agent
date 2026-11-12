// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/writeopen"
)

// PortableErrMsg returns a POSIX-style error message for the given error,
// normalizing platform-specific syscall messages to consistent strings.
// This ensures shell error output is identical across Linux, macOS, and Windows.
func PortableErrMsg(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, writeopen.ErrSymlinkWriteTarget):
		return writeopen.ErrSymlinkWriteTarget.Error()
	case errors.Is(err, writeopen.ErrNotRegularFile):
		return writeopen.ErrNotRegularFile.Error()
	case errors.Is(err, ErrMultiplyLinkedWriteTarget):
		return ErrMultiplyLinkedWriteTarget.Error()
	case errors.Is(err, fs.ErrNotExist):
		return "no such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrExist):
		return "file exists"
	case IsErrIsDirectory(err):
		return "is a directory"
	}
	return err.Error()
}

// rewrapPathError rebuilds err as a *os.PathError using the caller-facing op
// and path rather than whatever os.Root's internal error carried (e.g. its
// own "statat" op name and a path relative to the sandbox root, not the
// path the caller passed in). The inner error is preserved as-is (not
// stringified) so errors.Is checks against fs.ErrNotExist/fs.ErrPermission
// etc. still work when the result is passed through PortableErrMsg again.
func rewrapPathError(op, path string, err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return &os.PathError{Op: op, Path: path, Err: pe.Err}
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}

// PortablePathError returns a *os.PathError with a normalized error message.
// If the error is not a *os.PathError, it is returned as-is.
// Only the Err field is normalized; the Path and Op fields are preserved as-is.
func PortablePathError(err error) error {
	if err == nil {
		return nil
	}
	var pe *os.PathError
	if !errors.As(err, &pe) {
		return err
	}
	return &os.PathError{
		Op:   pe.Op,
		Path: pe.Path,
		Err:  errors.New(PortableErrMsg(pe.Err)),
	}
}
