// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package opener

import (
	"io"
	"os"

	"github.com/spf13/afero"
)

// MockFileInfo is a mock implementation of the os.FileInfo interface
type MockFileInfo struct {
	os.FileInfo
	size int64
}

// Size returns the size of the file
func (m *MockFileInfo) Size() int64 {
	return m.size
}

// MockFile is a mock implementation of the afero.File interface
type MockFile struct {
	afero.File
	readIdx      int
	fileContents fileContents // Data to return for each read
	currentPos   int64        // Track position for Seek
	name         string       // Name of the file
}

type fileContents struct {
	fileIdx   int
	outputs   [][][]byte
	fileSizes []int64
}

func newFileContents(outputs [][][]byte) fileContents {
	fileSizes := make([]int64, len(outputs))
	for fileIdx, output := range outputs {
		for _, output := range output {
			fileSizes[fileIdx] += int64(len(output))
		}
	}
	return fileContents{
		fileIdx:   0,
		outputs:   outputs,
		fileSizes: fileSizes,
	}
}

func (f *fileContents) getBytesAt(readIdx int) *[]byte {
	if f.fileIdx >= len(f.outputs) {
		return nil
	}
	fileContents := f.outputs[f.fileIdx]
	if readIdx >= len(fileContents) {
		return nil
	}
	data := fileContents[readIdx]
	return &data
}

func (f *fileContents) rotateIfDone(readIdx int) bool {
	if readIdx >= len(f.outputs[f.fileIdx]) && f.fileIdx < len(f.outputs)-1 {
		f.fileIdx++
		return true
	}
	return false
}

func (f *fileContents) getFileSize() int64 {
	return f.fileSizes[f.fileIdx]
}

// NewMockFile creates a new MockFile with the given expected outputs
func NewMockFile(name string, expectedOutputs ...[][]byte) *MockFile {
	fileSize := 0
	for _, output := range expectedOutputs {
		fileSize += len(output)
	}
	return &MockFile{
		name:         name,
		fileContents: newFileContents(expectedOutputs),
	}
}

// Name returns the name of the file
func (m *MockFile) Name() string {
	return m.name
}

// Path returns the path of the file
func (m *MockFile) Path() string {
	return m.name
}

// FileSize returns the size of the file
func (m *MockFile) FileSize() int {
	return int(m.fileContents.getFileSize())
}

// CurrentPos returns the current position of the file
func (m *MockFile) CurrentPos() int {
	return int(m.currentPos)
}

// Read reads data from the file
func (m *MockFile) Read(p []byte) (int, error) {
	data := m.fileContents.getBytesAt(m.readIdx)
	if data == nil {
		return 0, io.EOF
	}

	m.readIdx++
	n := copy(p, *data)
	m.currentPos += int64(n)

	didRotate := m.fileContents.rotateIfDone(m.readIdx)
	if didRotate {
		m.readIdx = 0
		m.currentPos = 0
	}

	return n, nil
}

// Stat returns the file info
func (m *MockFile) Stat() (os.FileInfo, error) {
	return &MockFileInfo{size: m.fileContents.getFileSize()}, nil
}

// Seek sets the position for the next Read
func (m *MockFile) Seek(offset int64, _ int) (int64, error) {
	m.currentPos = offset
	return offset, nil
}

// Close closes the file
func (m *MockFile) Close() error {
	return nil
}
