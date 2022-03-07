// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package file

import (
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.File.Path)
	if err != nil {
		return err
	}
	t.fullpath = fullpath

	// adds metadata to enable users to filter logs by filename
	t.tags = t.buildTailerTags()

	log.Info("Opening ", t.fullpath)
	f, err := openFile(t.fullpath)
	if err != nil {
		return err
	}
	filePos, _ := f.Seek(offset, whence)
	f.Close()

	t.readOffset = filePos
	t.decodedOffset = filePos

	return nil
}

func (t *Tailer) readAvailable() (int, error) {
	// If the file has already rotated, there is nothing to be done. Unlike on *nix,
	// there is no open file handle from which remaining data might be read.
	if t.hasFileRotated() {
		return 0, io.EOF
	}

	f, err := openFile(t.fullpath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		log.Debugf("Error stat()ing file %v", err)
		return 0, err
	}

	sz := st.Size()
	offset := t.GetReadOffset()
	if sz < offset {
		log.Debugf("File size of %s is shorter than last read offset; returning EOF", t.fullpath)
		return 0, io.EOF
	}

	f.Seek(offset, io.SeekStart)
	bytes := 0

	for {
		inBuf := make([]byte, 4096)
		n, err := f.Read(inBuf)
		bytes += n
		if n == 0 || err != nil {
			return bytes, err
		}
		t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
	}
}

// read lets the tailer tail the content of a file until it is closed. The
// windows version open and close the file between each call to 'read'. This is
// needed in order not to block the file and prevent the user from renaming it.
func (t *Tailer) read() (int, error) {
	n, err := t.readAvailable()
	if err == io.EOF || os.IsNotExist(err) {
		return n, nil
	} else if err != nil {
		t.File.Source.Status.Error(err)
		return n, log.Error("Err: ", err)
	}
	return n, nil
}
