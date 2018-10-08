// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	path, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.tags = []string{fmt.Sprintf("filename:%s", filepath.Base(t.path))}
	t.fullpath = path
	t.readOffset = offset
	t.decodedOffset = offset
	log.Info("Opening ", t.fullpath)
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

	for {
		inBuf := make([]byte, 4096)
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
	backoff := t.minWaitDuration
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			err := t.readAvailable()
			if isEOF(err) || os.IsNotExist(err) {
				if backoff > maxWaitDuration {
					// stop tailing the file as no data has been received or the file does not exist anymore,
					// a new tailer should be created later on by the scanner if the file still exists.
					log.Infof("No data has been written to %v for a while, stop reading", t.path)
				}
				t.wait(backoff)
				backoff *= 2
				continue
			}
			if err != nil {
				t.source.Status.Error(err)
				log.Error("Err: ", err)
				return
			}
			backoff = t.minWaitDuration
		}
	}
}
