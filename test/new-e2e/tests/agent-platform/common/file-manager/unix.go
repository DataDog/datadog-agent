// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filemanager implement interfaces to run install-script tests
package filemanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Unix implement filemanager interface for Unix distributions
type Unix struct {
	host *components.RemoteHost
}

// NewUnixFileManager create a new unix file manager
func NewUnixFileManager(host *components.RemoteHost) *Unix {
	return &Unix{host: host}
}

// FileExists check if the file exists, return an error if it does not
func (u *Unix) FileExists(path string) (string, error) {
	return u.host.Execute(fmt.Sprintf("sudo stat %s", path))
}

// ReadFile read the content of the file, return error if the file do not exists
func (u *Unix) ReadFile(path string) (string, error) {
	return u.host.Execute(fmt.Sprintf("sudo cat %s", path))
}

// FindFileInFolder search for files in the given folder return an error if no files are found
func (u *Unix) FindFileInFolder(path string) (string, error) {
	return u.host.Execute(fmt.Sprintf("sudo find %s -type f", path))
}

// WriteFile write content to the file
func (u *Unix) WriteFile(path string, content string) (string, error) {
	return u.host.Execute(fmt.Sprintf(`sudo bash -c " echo '%s' > %s"`, content, path))
}
