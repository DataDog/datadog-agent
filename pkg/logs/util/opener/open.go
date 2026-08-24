// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	internalOpener "github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FileOpener is an interface that defines the method to open a log file.
type FileOpener interface {
	OpenLogFile(path string) (afero.File, error)
	// OpenLogFileWithFlags opens a short-lived, read-only descriptor for
	// fingerprinting with the requested read-only open flags (e.g. O_DIRECT).
	// Callers must gate on Linux before invoking this; use OpenLogFile elsewhere.
	// On Linux it is a best-effort optimization scoped to files the Agent can open
	// directly; callers retry with OpenLogFile when flags are unsupported
	// (see IsOpenFlagsUnsupportedError).
	OpenLogFileWithFlags(path string, openFlags []types.FileOpenFlag) (afero.File, error)
	OpenShared(path string) (afero.File, error)
	Abs(path string) (string, error)
}

// NewFileOpener creates a new FileOpener
func NewFileOpener() FileOpener {
	return &fileOpenerImpl{}
}

// fileOpenerImpl is a struct that contains the default file opener implementation
type fileOpenerImpl struct {
}

// OpenLogFile utilizes an os-specific implementation to open a log file in a shared mode.
// On some operating systems, this will involve making an attempt to open the file via a privileged logs client.
// If the file is not intended to attempt privilege escalation for access (e.g. it is not a log file), then the OpenShared
// function should be used instead. This will minimize avoidable error logs for failed privilege escalation attempts.
func (f *fileOpenerImpl) OpenLogFile(path string) (afero.File, error) {
	return internalOpener.OpenLogFile(path)
}

// OpenLogFileWithFlags opens a short-lived fingerprint descriptor with the
// requested read-only open flags. Callers must invoke this only on Linux when
// flags are configured; use OpenLogFile on other platforms.
func (f *fileOpenerImpl) OpenLogFileWithFlags(path string, openFlags []types.FileOpenFlag) (afero.File, error) {
	direct := false
	for _, openFlag := range openFlags {
		switch openFlag {
		case types.FileOpenFlagDirect:
			direct = true
		default:
			// Unknown flags are ignored so a newer config can't break an older
			// Agent, but log it so misconfiguration is diagnosable.
			log.Debugf("ignoring unknown log open flag %q for %q", openFlag, path)
		}
	}
	if !direct {
		return nil, fmt.Errorf("OpenLogFileWithFlags: no supported open flags in %v", openFlags)
	}

	file, err := openDirect(path)
	if err != nil {
		// A file the Agent can't open directly (e.g. root-owned, tailed via the
		// privileged helper) can't get O_DIRECT anyway. Report it as unsupported
		// so the caller falls back to a buffered open and memoizes that decision,
		// instead of re-attempting the doomed direct open on every fingerprint.
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("%w: %w", ErrOpenFlagsUnsupported, err)
		}
		// Otherwise surface the error (e.g. filesystem rejected O_DIRECT) so the
		// caller can retry buffered via IsOpenFlagsUnsupportedError.
		return nil, err
	}
	return newDirectIOFile(file), nil
}

// OpenShared utilizes an os-specific implementation to open a generic file in a shared mode.
func (f *fileOpenerImpl) OpenShared(path string) (afero.File, error) {
	return filesystem.OpenShared(path)
}

// Abs returns the absolute path of the file (wrapper around filepath.Abs)
func (f *fileOpenerImpl) Abs(path string) (string, error) {
	return filepath.Abs(path)
}
