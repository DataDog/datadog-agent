// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filemanager implement interfaces to run install-script tests
package filemanager

import (
	"io/fs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// RemoteHost implement filemanager interface using components.RemoteHost
type RemoteHost struct {
	host *components.RemoteHost
}

// NewRemoteHost create a new file manager using components.RemoteHost
// Note: The file operations will be restricted to the permissions of the client connection user
func NewRemoteHost(host *components.RemoteHost) *RemoteHost {
	return &RemoteHost{host: host}
}

// FileExists check if the file exists, return an error if it does not
func (u *RemoteHost) FileExists(path string) (bool, error) {
	return u.host.FileExists(path)
}

// ReadFile read the content of the file, return error if the file do not exists
func (u *RemoteHost) ReadFile(path string) ([]byte, error) {
	return u.host.ReadFile(path)
}

// ReadDir returns list of directory entries in path
func (u *RemoteHost) ReadDir(path string) ([]fs.DirEntry, error) {
	return u.host.ReadDir(path)
}

// WriteFile write content to the file
func (u *RemoteHost) WriteFile(path string, content []byte) (int64, error) {
	return u.host.WriteFile(path, content)
}
