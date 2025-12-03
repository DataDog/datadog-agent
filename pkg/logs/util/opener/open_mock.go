// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package opener

import (
	"fmt"

	"github.com/spf13/afero"
)

// MockFileOpener is a mock implementation of the opener.Opener interface
type MockFileOpener struct {
	MockedFiles map[string]*MockFile
}

// NewMockFileOpener creates a new MockFileOpener
func NewMockFileOpener() *MockFileOpener {
	return &MockFileOpener{
		MockedFiles: make(map[string]*MockFile),
	}
}

// AddMockFile adds a mock file to the MockFileOpener
func (m *MockFileOpener) AddMockFile(file *MockFile) {
	m.MockedFiles[file.Name()] = file
}

// OpenShared returns the specified mock file or an error if the file was not added to the mock opener.
func (m *MockFileOpener) OpenShared(path string) (afero.File, error) {
	file, ok := m.MockedFiles[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return file, nil
}

// OpenLogFile returns the specified mock file or an error if the file was not added to the mock opener.
func (m *MockFileOpener) OpenLogFile(path string) (afero.File, error) {
	file, ok := m.MockedFiles[path]
	if !ok {
		return nil, fmt.Errorf("file not found: [ %s ]", path)
	}
	return file, nil
}

// Abs returns a mock path consisting of just the filename
func (m *MockFileOpener) Abs(path string) (string, error) {
	return path, nil
}
