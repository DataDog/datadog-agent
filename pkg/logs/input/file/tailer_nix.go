// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package file

import (
	"io"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.fullpath = fullpath

	// adds metadata to enable users to filter logs by filename
	t.tags = t.buildTailerTags()

	log.Info("Opening ", t.path)
	f, err := openFile(fullpath)
	if err != nil {
		return err
	}

	t.file = f
	ret, _ := f.Seek(offset, whence)
	t.readOffset = ret
	t.decodedOffset = ret

	return nil
}

// read lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) read() error {
	// keep reading data from file
	inBuf := make([]byte, 4096)
	n, err := t.file.Read(inBuf)
	if err != nil && err != io.EOF {
		// an unexpected error occurred, stop the tailor
		t.source.Status.Error(err)
		return log.Error("Unexpected error occurred while reading file: ", err)
	}
	if n == 0 {
		return nil
	}
	t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
	t.incrementReadOffset(n)
	return nil
}
