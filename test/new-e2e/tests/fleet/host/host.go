// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host contains host-level test helpers for fleet tests.
package host

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// Host wraps an environments.Host with helper methods for fleet tests.
type Host struct {
	*environments.Host
}

// New creates a new Host wrapper.
func New(host *environments.Host) *Host {
	return &Host{Host: host}
}

// FilePermissions represents the permissions of a file on Unix systems.
type FilePermissions struct {
	Mode  string
	Owner string
	Group string
}

// GetFilePermissions returns the permissions of a file on Unix systems.
// Returns an error on Windows as POSIX permissions don't apply.
func (h *Host) GetFilePermissions(filePath string) (*FilePermissions, error) {
	switch h.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		// Use stat to get file permissions, owner, and group
		output, err := h.RemoteHost.Execute("stat -c '%a %U %G' " + filePath)
		if err != nil {
			return nil, err
		}
		parts := strings.Fields(strings.TrimSpace(output))
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected stat output: %s", output)
		}
		return &FilePermissions{
			Mode:  parts[0],
			Owner: parts[1],
			Group: parts[2],
		}, nil
	case e2eos.WindowsFamily:
		// Windows doesn't use POSIX permissions
		return nil, errors.New("file permissions check not supported on Windows")
	default:
		return nil, fmt.Errorf("unsupported OS family: %v", h.RemoteHost.OSFamily)
	}
}

// DirExists checks if a directory exists on the remote host.
// Returns true if the path exists and is a directory, false if it doesn't exist or isn't a directory.
// Only returns an error for actual failures (e.g., permission issues).
func (h *Host) DirExists(path string) (bool, error) {
	info, err := h.RemoteHost.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}
