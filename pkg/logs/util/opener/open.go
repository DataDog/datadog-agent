// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/spf13/afero"

	internalOpener "github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// FileOpener is an interface that defines the method to open a log file.
type FileOpener interface {
	OpenLogFile(path string) (afero.File, error)
	// ReadDirectFingerprintRange opens path with the requested read-only flags
	// (e.g. O_DIRECT) and returns the logical bytes in [skip, skip+count). Only
	// the aligned range covering that interval is read from the kernel.
	ReadDirectFingerprintRange(path string, skip, count int, openFlags []types.FileOpenFlag) ([]byte, error)
	// OpenDirectFingerprintStream opens path with the requested read-only flags
	// and returns a forward-only reader over [0, limit). Callers must Close it.
	OpenDirectFingerprintStream(path string, limit int, openFlags []types.FileOpenFlag) (io.ReadCloser, error)
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

func (f *fileOpenerImpl) ReadDirectFingerprintRange(path string, skip, count int, openFlags []types.FileOpenFlag) ([]byte, error) {
	if err := requireDirectOpenFlags(openFlags); err != nil {
		return nil, err
	}
	return readDirectFingerprintRange(path, skip, count)
}

func (f *fileOpenerImpl) OpenDirectFingerprintStream(path string, limit int, openFlags []types.FileOpenFlag) (io.ReadCloser, error) {
	if err := requireDirectOpenFlags(openFlags); err != nil {
		return nil, err
	}
	file, err := openDirect(path)
	if err != nil {
		return nil, err
	}
	reader, err := openDirectFingerprintStream(file, limit)
	if err != nil {
		file.Close()
		return nil, err
	}
	return reader, nil
}

func requireDirectOpenFlags(openFlags []types.FileOpenFlag) error {
	if !slices.Contains(openFlags, types.FileOpenFlagDirect) {
		return fmt.Errorf("direct fingerprint read: no supported open flags in %v", openFlags)
	}
	return nil
}

// OpenShared utilizes an os-specific implementation to open a generic file in a shared mode.
func (f *fileOpenerImpl) OpenShared(path string) (afero.File, error) {
	return filesystem.OpenShared(path)
}

// Abs returns the absolute path of the file (wrapper around filepath.Abs)
func (f *fileOpenerImpl) Abs(path string) (string, error) {
	return filepath.Abs(path)
}
