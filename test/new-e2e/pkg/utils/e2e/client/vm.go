// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/executeparams"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
)

// VM is an interface that provides methods to run commands on a virtual machine.
type VM interface {
	// ExecuteWithError executes a command and returns an error if any.
	ExecuteWithError(command string, options ...executeparams.Option) (string, error)

	// Execute executes a command and returns its output.
	Execute(command string, options ...executeparams.Option) string

	// CopyFile copy file to the remote host
	CopyFile(src string, dst string)

	// CopyFolder copy a folder to the remote host
	CopyFolder(srcFolder string, dstFolder string)

	// GetOSType returns the OS type of the VM.
	GetOSType() componentos.Type

	// GetFile copy file from the remote host
	GetFile(src string, dst string) error

	// FileExists returns true if the file exists and is a regular file and returns an error if any
	FileExists(path string) (bool, error)

	// ReadFile reads the content of the file, return bytes read and error if any
	ReadFile(path string) ([]byte, error)

	// WriteFile write content to the file and returns the number of bytes written and error if any
	WriteFile(path string, content []byte) (int64, error)

	// ReadDir returns list of directory entries in path
	ReadDir(path string) ([]fs.DirEntry, error)

	// Lstat returns a FileInfo structure describing path.
	// if path is a symbolic link, the FileInfo structure describes the symbolic link.
	Lstat(path string) (fs.FileInfo, error)

	// MkdirAll creates the specified directory along with any necessary parents.
	// If the path is already a directory, does nothing and returns nil.
	// Otherwise returns an error if any.
	MkdirAll(path string) error

	// Remove removes the specified file or directory.
	// Returns an error if file or directory does not exist, or if the directory is not empty.
	Remove(path string) error

	// RemoveAll recursively removes all files/folders in the specified directory.
	// Returns an error if the directory does not exist.
	RemoveAll(path string) error

	// ReconnectSSH recreate the SSH connection to the VM. Should be used only after VM reboot to restore the SSH connection.
	// Returns an error if the VM is not reachable after retries.
	ReconnectSSH() error
}
