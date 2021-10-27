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

// Writer is an io.Writer implementation that scrubts content before writing to
// a target file.
type Writer struct {
	targetFile *os.File
	targetBuf  *bufio.Writer
	scrubber   *Scrubber
}

// newWriterWithScrubber creates a Writer.  Typically, this is accessed via sc.NewWriter for
// some scrubber sc.
func newWriterWithScrubber(path string, perms os.FileMode, scrubber *Scrubber) (*Writer, error) {
	targetFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, perms)
	if err != nil {
		return nil, err
	}

	targetBuf := bufio.NewWriter(targetFile)

	return &Writer{targetFile, targetBuf, scrubber}, nil
}

// WriteFromFile will read contents from file and write them scrubbed to target. If
// the file does not exist, this returns an error.
func (f *Writer) WriteFromFile(filePath string) (int, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	return f.Write(data)
}

// Write writes the scrubbed byte stream, applying all replacers and credential
// cleanup to target
func (f *Writer) Write(p []byte) (int, error) {
	cleaned, err := f.scrubber.ScrubBytes(p)
	if err != nil {
		return 0, err
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
