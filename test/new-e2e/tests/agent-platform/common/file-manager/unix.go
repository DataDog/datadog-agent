// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filemanager implement interfaces to run install-script tests
package filemanager

import (
	"fmt"
	"io/fs"
	"strings"

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
func (u *Unix) FileExists(path string) (bool, error) {
	out, err := u.host.Execute(fmt.Sprintf("sudo stat '%s'", path))
	if err != nil {
		return false, err
	}
	if len(out) == 0 {
		return false, fs.ErrNotExist
	}
	return true, nil
}

// dummy struct to partially convert find command output to DirEntry
type dummyentry struct {
	name string
}

func (e *dummyentry) Name() string {
	return e.name
}
func (e *dummyentry) IsDir() bool {
	panic(fmt.Errorf("not implemented"))
}
func (e *dummyentry) Type() fs.FileMode {
	panic(fmt.Errorf("not implemented"))
}
func (e *dummyentry) Info() (fs.FileInfo, error) {
	panic(fmt.Errorf("not implemented"))
}

// ReadDir only returns the Name of files in path, not stat modes
// TODO: Return a real DirEntry
func (u *Unix) ReadDir(path string) ([]fs.DirEntry, error) {
	out, err := u.host.Execute(fmt.Sprintf("sudo find '%s'", path))
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fs.ErrNotExist
	}

	entryNames := strings.Split(out, "\n")
	entries := make([]fs.DirEntry, 0, len(entryNames))
	for _, name := range entryNames {
		entries = append(entries, &dummyentry{name: name})
	}
	return entries, nil
}

// ReadFile read the content of the file, return error if the file do not exists
func (u *Unix) ReadFile(path string) ([]byte, error) {
	out, err := u.host.Execute(fmt.Sprintf("sudo cat '%s'", path))
	return []byte(out), err
}

// WriteFile write content to the file, does not return number of bytes written
// TODO: return number of bytes written
func (u *Unix) WriteFile(path string, content []byte) (int64, error) {
	_, err := u.host.Execute(fmt.Sprintf(`sudo bash -c " echo '%s' > '%s'"`, content, path))
	if err != nil {
		return 0, err
	}
	return 0, nil
}
