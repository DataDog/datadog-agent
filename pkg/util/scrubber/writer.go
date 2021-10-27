// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
)

// BUG(dustin) the writer applies scrubbing to each "chunk" of data independently. If
// a sensitive value spans two chunks, it will not be matched by a replacer and thus
// not scrubbed.

// Writer is an io.Writer implementation that redacts content before writing to
// target.
type Writer struct {
	targetFile *os.File
	targetBuf  *bufio.Writer
	r          []Replacer
}

// NewWriter instantiates a Writer to the given file path with the given
// permissions.
func NewWriter(path string, p os.FileMode) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, p)
	if err != nil {
		return nil, err
	}

	b := bufio.NewWriter(f)

	return &Writer{
		targetFile: f,
		targetBuf:  b,
		r:          []Replacer{},
	}, nil
}

// RegisterReplacer register additional replacers to run on stream
func (f *Writer) RegisterReplacer(r Replacer) {
	f.r = append(f.r, r)
}

// WriteFromFile will read contents from file and write them redacted to target. If
// the file does not exist, this returns an error.
func (f *Writer) WriteFromFile(filePath string) (int, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	return f.Write(data)
}

// Write writes the redacted byte stream, applying all replacers and credential
// cleanup to target
func (f *Writer) Write(p []byte) (int, error) {
	cleaned, err := ScrubBytes(p)
	if err != nil {
		return 0, err
	}

	for _, r := range f.r {
		if r.Regex != nil && r.ReplFunc != nil {
			cleaned = r.Regex.ReplaceAllFunc(cleaned, r.ReplFunc)
		}
	}

	n, err := f.targetBuf.Write(cleaned)

	if n != len(cleaned) {
		err = io.ErrShortWrite
	}

	return len(p), err
}

// Flush if this is a buffered writer, it flushes the buffer, otherwise NOP
func (f *Writer) Flush() error {
	return f.targetBuf.Flush()
}

// Close closes the underlying file, if buffered previously flushes the contents
func (f *Writer) Close() error {
	err := f.Flush()
	if err != nil {
		return err
	}

	return f.targetFile.Close()
}
