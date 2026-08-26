// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	internalOpener "github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrOpenFlagsUnsupported can be returned by platform openers when a filesystem
// rejects the requested read-only open flags.
var ErrOpenFlagsUnsupported = errors.New("requested log open flags are not supported")

// IsOpenFlagsUnsupportedError reports whether err means the configured
// open_flags cannot be honoured for this file, as opposed to an ordinary I/O
// failure. The condition is a property of the file and its filesystem, so it
// does not clear on retry and is surfaced to the operator instead.
func IsOpenFlagsUnsupportedError(err error) bool {
	return errors.Is(err, ErrOpenFlagsUnsupported) || isOpenFlagsUnsupportedError(err)
}

// FileOpener is an interface that defines the method to open a log file.
type FileOpener interface {
	OpenLogFile(path string) (afero.File, error)
	// OpenReaderWithFlags opens path read-only through a short-lived descriptor
	// carrying the requested flags (e.g. O_DIRECT) and returns a reader over it.
	// Reads and seeks are lazy, so a fingerprint that skips far into a large file
	// only touches the blocks it hashes. The caller owns the reader and must
	// Close it.
	// Callers must gate on Linux before invoking this; use OpenLogFile elsewhere.
	// It only works for files the Agent can open directly, and returns an error
	// matching IsOpenFlagsUnsupportedError when the requested flags cannot be
	// honoured.
	OpenReaderWithFlags(path string, openFlags []types.FileOpenFlag) (io.ReadSeekCloser, error)
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

// OpenReaderWithFlags opens path through a short-lived fingerprint descriptor
// carrying the requested read-only open flags, and returns a lazy reader over
// it. Callers must invoke this only on Linux when flags are configured; use
// OpenLogFile on other platforms.
func (f *fileOpenerImpl) OpenReaderWithFlags(path string, openFlags []types.FileOpenFlag) (io.ReadSeekCloser, error) {
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
		return nil, fmt.Errorf("OpenReaderWithFlags: no supported open flags in %v", openFlags)
	}

	file, err := openDirect(path)
	if err != nil {
		// A file the Agent can't open directly (e.g. root-owned, tailed via the
		// privileged helper) can never get O_DIRECT, which is a property of the
		// deployment rather than a transient failure. Mark it as such so the
		// caller can tell it apart from an ordinary open error.
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("%w: %w", ErrOpenFlagsUnsupported, err)
		}
		// Other errors (e.g. the filesystem rejecting O_DIRECT with EINVAL) are
		// classified by the platform-specific isOpenFlagsUnsupportedError.
		return nil, err
	}

	reader, err := newDirectReader(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	return reader, nil
}

// OpenShared utilizes an os-specific implementation to open a generic file in a shared mode.
func (f *fileOpenerImpl) OpenShared(path string) (afero.File, error) {
	return filesystem.OpenShared(path)
}

// Abs returns the absolute path of the file (wrapper around filepath.Abs)
func (f *fileOpenerImpl) Abs(path string) (string, error) {
	return filepath.Abs(path)
}
