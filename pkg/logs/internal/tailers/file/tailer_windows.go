// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package file

import (
	"io"
	"os"
	"path/filepath"
	"time"

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

	log.Info("Opening ", t.fullpath)
	f, err := filesystem.OpenShared(t.fullpath)
	if err != nil {
		return err
	}
	filePos, _ := f.Seek(offset, whence)
	f.Close()

	t.lastReadOffset.Store(filePos)
	t.decodedOffset.Store(filePos)

	return nil
}

func (t *Tailer) readAvailable() (int, error) {
	// If the file has already rotated, there is nothing to be done. Unlike on *nix,
	// there is no open file handle from which remaining data might be read.
	if t.didFileRotate.Load() {
		return 0, io.EOF
	}

	var f *os.File
	defer func() {
		if f != nil {
			f.Close()
		}
	}()

	bytes := 0
	for {
		if f == nil {
			var err error
			f, err = filesystem.OpenShared(t.fullpath)
			if err != nil {
				return bytes, err
			}
			st, err := f.Stat()
			if err != nil {
				log.Debugf("Error stat()ing file %v", err)
				return bytes, err
			}

			sz := st.Size()
			offset := t.lastReadOffset.Load()
			if sz < offset {
				log.Debugf("File size of %s is shorter than last read offset; returning EOF", t.fullpath)
				return bytes, io.EOF
			}

			_, err = f.Seek(offset, io.SeekStart)
			if err != nil {
				log.Debugf("Error seek()ing file %v", err)
				return bytes, err
			}
		}

		inBuf := make([]byte, 4096)
		n, err := f.Read(inBuf)
		bytes += n
		if n == 0 || err != nil {
			return bytes, err
		}

		// First, try to send the data to the decoder, but only wait for
		// windowsOpenFileTimeout.  This short-term blocking send allows this
		// component to hold a file open over any short-term blockages in the
		// logs pipeline.
		timer := time.NewTimer(t.windowsOpenFileTimeout)
		select {
		case t.decoder.InputChan <- decoder.NewInput(inBuf[:n]):
			timer.Stop()
		case <-timer.C:
			// The windowsOpenFileTimeout expired, and we want to avoid
			// blocking with the file open. So close the file before performing
			// a blocking send.  The file will be re-opened on the next
			// iteration, after the send succeeds.  NOTE: if the open file has
			// been rotated, then the re-open will access a different file and
			// any remaining data in the rotated file will not be seen.
			f.Close()
			f = nil

			// blocking send to the decoder
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		}

		// record these bytes as having been read
		t.lastReadOffset.Add(int64(n))
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
		t.file.Source.Status().Error(err)
		return n, log.Error("Err: ", err)
	}
	return n, nil
}
