// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package file

import (
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.tags = []string{fmt.Sprintf("filename:%s", filepath.Base(t.path))}
	log.Info("Opening ", t.path)
	f, err := os.Open(fullpath)
	if err != nil {
		return err
	}

	t.file = f
	ret, _ := f.Seek(offset, whence)
	t.readOffset = ret
	t.decodedOffset = ret

	return nil
}

// readForever lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) readForever() {
	defer t.onStop()
	backoff := t.minWaitDuration
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			// keep reading data from file
			inBuf := make([]byte, 4096)
			n, err := t.file.Read(inBuf)
			switch {
			case err != nil && !isEOF(err):
				// an unexpected error occurred, stop the tailor
				t.source.Status.Error(err)
				log.Errorf("Could not read logs from file %v: %v", t.path, err)
				return
			case n == 0:
				// wait for new data to come, retry exponentially until the max limit is reached
				if backoff > maxWaitDuration {
					// stop tailing the file as no data has been received  or the file does not exist anymore,
					// a new tailer should be created later on by the scanner if the file still exists.
					log.Infof("No data has been written to %v for a while, stop reading", t.path)
					return
				}
				t.wait(backoff)
				backoff *= 2
				continue
			default:
				backoff = t.minWaitDuration
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
			t.incrementReadOffset(n)
		}
	}
}

// isEOF returns true if the error occured because of an EOF.
func isEOF(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
