// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package opener

import (
	"fmt"
	"io"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// MockFileOpener is a mock implementation of the opener.Opener interface
type MockFileOpener struct {
	MockedFiles map[string]*MockFile
	OpenCalls   [][]types.FileOpenFlag
	OpenErrors  []error
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
	return m.openLogFile(path, nil)
}

func (m *MockFileOpener) ReadDirectFingerprintRange(path string, skip, count int, openFlags []types.FileOpenFlag) ([]byte, error) {
	file, err := m.openLogFile(path, openFlags)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if skip > 0 {
		if _, err := file.Seek(int64(skip), io.SeekStart); err != nil {
			return nil, err
		}
	}
	buffer := make([]byte, count)
	read, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buffer[:read], nil
}

func (m *MockFileOpener) OpenDirectFingerprintStream(path string, limit int, openFlags []types.FileOpenFlag) (io.ReadCloser, error) {
	file, err := m.openLogFile(path, openFlags)
	if err != nil {
		return nil, err
	}
	return &limitedMockReader{Reader: io.LimitReader(file, int64(limit)), closer: file}, nil
}

type limitedMockReader struct {
	io.Reader
	closer io.Closer
}

func (r *limitedMockReader) Close() error {
	return r.closer.Close()
}

func (m *MockFileOpener) openLogFile(path string, openFlags []types.FileOpenFlag) (afero.File, error) {
	m.OpenCalls = append(m.OpenCalls, append([]types.FileOpenFlag(nil), openFlags...))
	if len(m.OpenErrors) > 0 {
		err := m.OpenErrors[0]
		m.OpenErrors = m.OpenErrors[1:]
		if err != nil {
			return nil, err
		}
	}
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
