// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"

	internalOpener "github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// SymlinkPolicy controls whether OpenLogFile follows or rejects symbolic links.
type SymlinkPolicy int

const (
	// symlinkPolicyInvalid is the zero value; callers must always choose an explicit policy.
	symlinkPolicyInvalid SymlinkPolicy = iota
	// FollowSymlinks resolves symbolic links when opening a log file.  Use for
	// file sources whose paths are admin-specified or come from the container
	// runtime (e.g. /var/log/pods/…).
	FollowSymlinks
	// RejectSymlinks opens every path component with O_NOFOLLOW so that any
	// symlink encountered causes an immediate error.  Use for file sources whose
	// paths are resolved from /proc/<pid>/fd by the process_log provider: those
	// paths are canonical at discovery time, so a symlink found later indicates
	// an attacker-controlled swap.
	RejectSymlinks
)

// FileOpener is an interface that defines the method to open a log file.
type FileOpener interface {
	// OpenLogFile opens a log file using the given symlink policy.  Callers must
	// explicitly pass either FollowSymlinks or RejectSymlinks; passing the zero
	// value causes an error so that new call sites cannot silently get the wrong
	// behaviour.
	OpenLogFile(path string, policy SymlinkPolicy) (afero.File, error)
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
//
// The caller must provide an explicit SymlinkPolicy; passing the zero value returns an error.
func (f *fileOpenerImpl) OpenLogFile(path string, policy SymlinkPolicy) (afero.File, error) {
	switch policy {
	case FollowSymlinks:
		return internalOpener.OpenLogFile(path)
	case RejectSymlinks:
		return internalOpener.OpenLogFileNoFollow(path)
	default:
		return nil, fmt.Errorf("opener: invalid SymlinkPolicy %d; must be FollowSymlinks or RejectSymlinks", policy)
	}
}

// OpenShared utilizes an os-specific implementation to open a generic file in a shared mode.
func (f *fileOpenerImpl) OpenShared(path string) (afero.File, error) {
	return filesystem.OpenShared(path)
}

// Abs returns the absolute path of the file (wrapper around filepath.Abs)
func (f *fileOpenerImpl) Abs(path string) (string, error) {
	return filepath.Abs(path)
}
