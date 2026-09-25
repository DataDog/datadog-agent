// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package opener

import (
	"fmt"
	"sync"

	"github.com/spf13/afero"
)

// LogFileOpen records a call to OpenLogFile.
type LogFileOpen struct {
	Path     string
	NoFollow bool
}

type mockFileOpenerState struct {
	sync.Mutex
	files map[string]*MockFile
	opens []LogFileOpen
}

// MockFileOpener is a mock implementation of the opener.Opener interface
type MockFileOpener struct {
	state    *mockFileOpenerState
	noFollow bool
}

// NewMockFileOpener creates a new MockFileOpener
func NewMockFileOpener() *MockFileOpener {
	return &MockFileOpener{
		state: &mockFileOpenerState{files: make(map[string]*MockFile)},
	}
}

// AddMockFile adds a mock file to the MockFileOpener
func (m *MockFileOpener) AddMockFile(file *MockFile) {
	m.state.Lock()
	defer m.state.Unlock()
	m.state.files[file.Name()] = file
}

// OpenShared returns the specified mock file or an error if the file was not added to the mock opener.
func (m *MockFileOpener) OpenShared(path string) (afero.File, error) {
	m.state.Lock()
	defer m.state.Unlock()
	file, ok := m.state.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return file, nil
}

// OpenLogFile returns the specified mock file or an error if the file was not added to the mock opener.
func (m *MockFileOpener) OpenLogFile(path string) (afero.File, error) {
	m.state.Lock()
	defer m.state.Unlock()
	m.state.opens = append(m.state.opens, LogFileOpen{Path: path, NoFollow: m.noFollow})
	file, ok := m.state.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: [ %s ]", path)
	}
	return file, nil
}

// NoFollow returns a mock opener that records its opens as no-follow.
func (m *MockFileOpener) NoFollow() FileOpener {
	if m.noFollow {
		return m
	}
	return &MockFileOpener{
		state:    m.state,
		noFollow: true,
	}
}

// Opens returns the log-file opens recorded by this opener and its variants.
func (m *MockFileOpener) Opens() []LogFileOpen {
	m.state.Lock()
	defer m.state.Unlock()
	return append([]LogFileOpen(nil), m.state.opens...)
}

// ResetOpens clears the recorded log-file opens for this opener and its variants.
func (m *MockFileOpener) ResetOpens() {
	m.state.Lock()
	defer m.state.Unlock()
	m.state.opens = nil
}

// Abs returns a mock path consisting of just the filename
func (m *MockFileOpener) Abs(path string) (string, error) {
	return path, nil
}
