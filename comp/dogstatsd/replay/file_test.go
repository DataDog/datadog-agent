// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package replay

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderFormat(t *testing.T) {
	buff := bytes.NewBuffer([]byte{})
	contents := bufio.NewWriter(buff)

	err := WriteHeader(contents)
	assert.NoError(t, err)

	// let's make sure these are written to the underlying byte buffer
	contents.Flush()

	// it should match the file-format
	b := buff.Bytes()
	assert.True(t, datadogMatcher(b))

	// look at version
	v, err := fileVersion(b)
	assert.NoError(t, err)
	assert.Equal(t, v, int(datadogFileVersion))

	// let's inspect the header
	for i := 0; i < len(datadogHeader); i++ {
		if i != versionIndex {
			assert.Equal(t, b[i], datadogHeader[i])
		} else {
			assert.Equal(t, b[i], datadogHeader[i]|datadogFileVersion)
		}
	}
}

func TestHeaderFormatError(t *testing.T) {
	tests := []struct {
		name     string
		contents io.Writer
		expected error
	}{
		{
			name:     "No error but less bytes written than datadogHeader",
			contents: &errorWriter{1, nil},
			expected: ErrHeaderWrite,
		},
		{
			name:     "Error and less bytes written than datadogHeader",
			contents: &errorWriter{1, fs.ErrInvalid},
			expected: fs.ErrInvalid,
		},
		{
			name:     "Error and more bytes written than datadogHeader",
			contents: &errorWriter{500, fs.ErrInvalid},
			expected: fs.ErrInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, WriteHeader(tt.contents), tt.expected, tt.name)
		})
	}
}

type errorWriter struct {
	n   int
	err error
}

func (e *errorWriter) Write(_ []byte) (n int, err error) {
	return e.n, e.err
}

func TestFormatMatcher(t *testing.T) {
	assert.True(t, datadogMatcher(datadogHeader))

	badDatadogHeader := []byte{0xD4, 0x74, 0xD0, 0x66, 0xF0, 0xFF, 0x00, 0x00}
	assert.False(t, datadogMatcher(badDatadogHeader))
}
