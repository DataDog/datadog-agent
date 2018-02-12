// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package tailer

import (
	"io"
	"os"
	"path/filepath"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	path, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	log.Info("Opening ", t.fullpath)

	t.fullpath = path
	t.readOffset = offset
	t.decodedOffset = offset

	return nil
}

func (t *Tailer) readAvailable() (err error) {
	err = nil
	f, err := os.Open(t.fullpath)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err == nil {
		sz := st.Size()
		log.Debugf("Size is %d, offset is %d", sz, t.GetReadOffset())
		if sz == 0 {
			log.Debug("File size now zero, resetting offset")
			t.SetReadOffset(0)
			t.SetDecodedOffset(0)
		} else if sz < t.GetReadOffset() {
			log.Debug("Offset off end of file, resetting")
			t.SetReadOffset(0)
			t.SetDecodedOffset(0)
		}
	} else {
		log.Debugf("Error stat()ing file %v", err)
		return err
	}
	f.Seek(t.GetReadOffset(), io.SeekStart)
	inBuf := make([]byte, 4096)

	for {
		n, err := f.Read(inBuf)
		if n == 0 || err != nil {
			log.Debugf("Done reading")
			return err
		}
		log.Debugf("Sending %d bytes to input channel", n)
		t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
	}
}

// readForever lets the tailer tail the content of a file
// until it is closed.
func (t *Tailer) readForever() {
	defer t.onStop()
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			err := t.readAvailable()
			if err == io.EOF || os.IsNotExist(err) {
				t.wait()
				continue
			}
			if err != nil {
				t.source.Status.Error(err)
				log.Error("Err: ", err)
				return
			}
		}
	}
}

// checkForRotation does nothing on windows, log rotations
// are for now handled by the readAvailable method
func (t *Tailer) checkForRotation() (bool, error) {
	return false, nil
}
