// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package file

import (
	"io"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.file.Path)
	if err != nil {
		return err
	}
	t.fullpath = fullpath

	// adds metadata to enable users to filter logs by filename
	t.tags = t.buildTailerTags()

	log.Info("Opening", t.file.Path, "for tailer key", t.file.GetScanKey())
	f, err := filesystem.OpenShared(fullpath)
	if err != nil {
		return err
	}

	t.osFile = f
	ret, _ := f.Seek(offset, whence)
	t.lastReadOffset.Store(ret)
	t.decodedOffset.Store(ret)

	return nil
}

// read lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) read() (int, error) {
	// keep reading data from file
	inBuf := make([]byte, 4096)
	n, err := t.osFile.Read(inBuf)
	if err != nil && err != io.EOF {
		// an unexpected error occurred, stop the tailor
		t.file.Source.Status().Error(err)
		return 0, log.Error("Unexpected error occurred while reading file: ", err)
	}
	if n == 0 {
		return 0, nil
	}
	t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
	t.lastReadOffset.Add(int64(n))
	return n, nil
}
